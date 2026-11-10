package tun

import (
	"errors"
	"fmt"
)

// These operations are also exercised with isolated test doubles. No live
// system proxy, privileged process, DNS or route is needed to test rollback.
type handoffActions struct {
	prepare, startTunnel, confirmTunnel       func() error
	stopNormal                                func() (bool, error)
	cancelTunnel, restoreNormal, disableProxy func() error
}

func handoff(wasRunning bool, actions handoffActions) (result error) {
	interrupted := false
	defer func() {
		if result == nil {
			return
		}
		if err := actions.cancelTunnel(); err != nil {
			result = errors.Join(result, fmt.Errorf("TUN 清理失败：%w", err))
			return
		}
		if !interrupted {
			if wasRunning {
				result = fmt.Errorf("未开启 TUN，原普通代理未停止：%w", result)
			}
			return
		}
		if err := actions.restoreNormal(); err == nil {
			result = fmt.Errorf("TUN 未能开启，已确认普通代理恢复：%w", result)
		} else {
			result = errors.Join(result, fmt.Errorf("普通代理恢复失败：%w", err))
			if err := actions.disableProxy(); err != nil {
				result = errors.Join(result, fmt.Errorf("关闭系统代理失败：%w", err))
			} else {
				result = fmt.Errorf("%w；已关闭系统代理，请检查当前网络后重试", result)
			}
		}
	}()
	if err := actions.prepare(); err != nil {
		return err
	}
	if wasRunning {
		var err error
		interrupted, err = actions.stopNormal()
		if err != nil {
			return err
		}
	}
	if err := actions.startTunnel(); err != nil {
		return err
	}
	return actions.confirmTunnel()
}
