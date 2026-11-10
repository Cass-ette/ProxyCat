package tun

// Progress reports completed handoff boundaries, rather than estimated percent.
// It deliberately contains no subscription URL, credentials or node names.
type Progress struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type ProgressFunc func(Progress)

func (report ProgressFunc) send(stage, message string) {
	if report != nil {
		report(Progress{Stage: stage, Message: message})
	}
}
