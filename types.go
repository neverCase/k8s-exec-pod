package k8s_exec_pod

type HttpRequest struct {
}

type HttpResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

type XtermMsg struct {
	MsgType string `json:"type"`
	Input   string `json:"input"`
	Rows    uint16 `json:"rows"`
	Cols    uint16 `json:"cols"`
}

const XtermMsgTypeResize = "resize"
const XtermMsgTypeInput = "input"
