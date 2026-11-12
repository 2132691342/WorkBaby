package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// chatState 聊天页首屏总装。
func (s *Server) chatState(c *gin.Context) {
	pkg.OK(c, s.svc.Chat.State(strings.TrimSpace(c.Query("session"))))
}

// chatSend 发消息。
func (s *Server) chatSend(c *gin.Context) {
	var req domain.ChatSendREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Chat.Send(req.SessionFile, req.Message, req.Attachments)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// chatStop 中止当前运行。
func (s *Server) chatStop(c *gin.Context) {
	if err := s.svc.Chat.Stop(); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatSteer 插话。
func (s *Server) chatSteer(c *gin.Context) {
	var req domain.ChatSendREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.Steer(req.Message, req.Attachments); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatFollowUp 追加排队。
func (s *Server) chatFollowUp(c *gin.Context) {
	var req domain.ChatSendREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.FollowUp(req.Message, req.Attachments); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatClearQueue 清空排队。
func (s *Server) chatClearQueue(c *gin.Context) {
	pkg.OK(c, s.svc.Chat.ClearQueue())
}

// chatRemoveQueued 单条移除排队消息，返回移除后的两条队。
func (s *Server) chatRemoveQueued(c *gin.Context) {
	var req domain.ChatRemoveQueuedREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Chat.RemoveQueued(req.QueueKind, req.QueueIndex)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// chatCompact 手动压缩。
func (s *Server) chatCompact(c *gin.Context) {
	var req domain.ChatCompactREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.Compact(req.SessionFile, req.Instructions); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatAnswer 回答提问卡。
func (s *Server) chatAnswer(c *gin.Context) {
	var req domain.ChatAnswerREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.Answer(req); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatNew 新建会话。
func (s *Server) chatNew(c *gin.Context) {
	file, err := s.svc.Chat.NewSession()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"session_file": file})
}

// chatClone 克隆当前会话：把已有条目复制进一份新会话，往后走独立的分支。
func (s *Server) chatClone(c *gin.Context) {
	file, err := s.svc.Chat.CloneSession()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"session_file": file})
}

// chatExport 导出当前会话为 HTML。
func (s *Server) chatExport(c *gin.Context) {
	var req domain.ChatSessionREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Chat.Export(req.SessionFile)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// chatModel 切模型。
func (s *Server) chatModel(c *gin.Context) {
	var req domain.ChatModelREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.SetModel(req.SessionFile, req.ProviderName, req.ModelId); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatThinking 切思考档。
func (s *Server) chatThinking(c *gin.Context) {
	var req domain.ChatThinkingREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.SetThinking(req.SessionFile, req.ThinkingLevel); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatAutocompact 自动压缩开关。
func (s *Server) chatAutocompact(c *gin.Context) {
	var req domain.ChatAutoCompactREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Chat.SetAutoCompaction(req.SessionFile, req.AutoCompaction); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// chatPlan 开关计划模式。
func (s *Server) chatPlan(c *gin.Context) {
	var req domain.ChatPlanREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Chat.TogglePlan(req.PlanEnabled)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// messageHide 隐藏一轮。
func (s *Server) messageHide(c *gin.Context) {
	var req domain.MessageHideREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Sessions.Hide(req.SessionFile, req.TurnId); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// messageUnhide 取消隐藏一轮。
func (s *Server) messageUnhide(c *gin.Context) {
	var req domain.MessageHideREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Sessions.Unhide(req.SessionFile, req.TurnId); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}
