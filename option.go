package k8s_exec_pod

import (
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExecOptions passed to ExecWithOptions
type ExecOptions struct {
	Command       []string
	Namespace     string
	PodName       string
	ContainerName string

	Follow          bool
	UsePreviousLogs bool
	SinceSeconds    *int64
	SinceTime       *metav1.Time

	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}
