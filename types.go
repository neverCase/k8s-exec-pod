package k8s_exec_pod

import "k8s.io/client-go/tools/remotecommand"

type HttpRequest struct {
}

type HttpResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

type TermMsg struct {
	MsgType TermMessageType `json:"type"`
	Input   string          `json:"input"`
	Rows    uint16          `json:"rows"`
	Cols    uint16          `json:"cols"`
}

type TermMessageType string

const (
	TermResize TermMessageType = "resize"
	TermInput  TermMessageType = "input"
	TermPing   TermMessageType = "ping"
)

// TerminalSession implements PtyHandler (using a SockJS connection)
type TerminalSession struct {
	id               string
	bound            chan error
	websocketSession Proxy
	sizeChan         chan remotecommand.TerminalSize
	doneChan         chan struct{}
}

// TerminalMessage is the messaging protocol between ShellController and TerminalSession.
//
// OP      DIRECTION  FIELD(S) USED  DESCRIPTION
// ---------------------------------------------------------------------
// bind    fe->be     SessionID      Id sent back from TerminalResponse
// stdin   fe->be     Data           Keystrokes/paste buffer
// resize  fe->be     Rows, Cols     New terminal size
// stdout  be->fe     Data           Output from the process
// toast   be->fe     Data           OOB message to be shown to the user
type TerminalMessage struct {
	Op, Data, SessionID string
	Rows, Cols          uint16
}
