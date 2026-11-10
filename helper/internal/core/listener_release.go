package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"
)

// WaitForListenerRelease is called after the privileged TUN process exits,
// before starting an ordinary user-owned core. Process exit does not prove
// that macOS will allow another user to bind its former TCP endpoints yet.
// Do not create a core that can survive failed binds with no controller.
func WaitForListenerRelease(config string, timeout time.Duration) error {
	settings, err := readListenerSettings(config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return waitForListenerRelease(ctx, []string{
		net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.MixedPort)),
		settings.Controller,
	})
}

func waitForListenerRelease(ctx context.Context, addresses []string) error {
	for {
		var pending error
		for _, address := range addresses {
			listener, err := net.Listen("tcp", address)
			if err == nil {
				listener.Close()
				continue
			}
			if !errors.Is(err, syscall.EADDRINUSE) {
				return fmt.Errorf("无法准备普通代理监听 %s：%w", address, err)
			}
			pending = fmt.Errorf("普通代理端口 %s 尚不能重新绑定：%w", address, err)
			break
		}
		if pending == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 TUN 释放本地端口超时：%w", pending)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
