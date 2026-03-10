package model

type EnforceAction string

const (
	EnforceActionRead   EnforceAction = "read"
	EnforceActionWrite  EnforceAction = "write"
	EnforceActionDelete EnforceAction = "delete"
	EnforceActionCreate EnforceAction = "create"
	EnforceActionUpdate EnforceAction = "update"
	EnforceActionManage EnforceAction = "manage"
	EnforceActionInvite EnforceAction = "invite"
	EnforceActionKick   EnforceAction = "kick"
)

// EnforceRequest - запрос на проверку доступа.
type EnforceRequest struct {
	Sub string
	Obj string
	Act EnforceAction
}

// EnforceResponse - ответ на запрос на проверку доступа.
type EnforceResponse struct {
	CanRead bool
	CanEdit bool
}

type EnforceNoteResponse struct {
	EnforceResponse
}
