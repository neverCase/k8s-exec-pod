package k8s_exec_pod

const (
	RouterPodShellToken  = "/namespace/:namespace/pod/:pod/shell/:container/:command"
	RouterSSH            = "/ssh/:token"
	RouterPodLogStream   = "/log/sinceSeconds/:SinceSeconds/sinceTime/:SinceTime/token/:token"
	RouterPodLogDownload = "/namespace/:namespace/pod/:pod/container/:container/previous/:previous/sinceSeconds/:SinceSeconds/sinceTime/:SinceTime"
)
