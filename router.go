package k8s_exec_pod

const (
	RouterPodShellToken  = "/namespace/:namespace/pod/:pod/shell/:container/:command"
	RouterSSH            = "/ssh/:token"
	RouterLog            = "/log/:token"
	RouterPodLogDownload = "/namespace/:namespace/pod/:pod/:container/previous/:previous/SinceSeconds/:SinceSeconds/SinceTime/:sinceTime"
)
