package selfupdate

import "errors"

const networkRetryHint = "请检查网络；部分网络需要开启代理或 VPN 后重试。"

// Keep recovery advice readable while preserving the original request error
// separately for diagnostics and the app's expandable details.
type requestError struct {
	message string
	cause   error
}

func (e *requestError) Error() string { return e.message }
func (e *requestError) Unwrap() error { return e.cause }

func failureEvent(prefix string, err error) Event {
	event := Event{Stage: "error", Message: prefix + err.Error()}
	var request *requestError
	if errors.As(err, &request) {
		event.Details = request.cause.Error()
	}
	return event
}
