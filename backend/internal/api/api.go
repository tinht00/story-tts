package api

import (
	"net/http"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/gin-gonic/gin"

	"story-tts/backend/internal/model"
	"story-tts/backend/internal/service"
)

type Server struct {
	manager *service.Manager
}

func NewRouter(manager *service.Manager) *gin.Engine {
	server := &Server{manager: manager}
	router := gin.Default()
	router.GET("/health", server.health)
	router.Static("/library", manager.LibraryDir())

	api := router.Group("/api")
	{
		api.GET("/state", server.getState)
		api.GET("/stories", server.listStories)
		api.GET("/library/stories", server.listStories)
		api.GET("/library/stories/:id", server.getStory)
		api.POST("/library/import-folder", server.importFolder)
		api.GET("/library/chapters/:id/content", server.getChapterContent)
		api.GET("/reader/progress/:storyId", server.getReaderProgress)
		api.POST("/reader/progress", server.saveReaderProgress)
		api.POST("/reader/edge-read-aloud/toggle", server.toggleEdgeReadAloud)
		api.POST("/stories/scan", server.scanStory)
		api.GET("/stories/:id", server.getStory)
		api.POST("/stories/:id/build", server.buildStory)
		api.POST("/chapters/:id/build", server.buildChapter)
		api.GET("/jobs", server.listJobs)
		api.GET("/jobs/:id", server.getJob)
		api.GET("/bot-profiles", server.listBotProfiles)
		api.POST("/bot-profiles", server.saveBotProfile)
		api.POST("/telegram/send-code", server.sendTelegramCode)
		api.POST("/telegram/sign-in", server.signInTelegram)
		api.POST("/telegram/password", server.telegramPassword)
		api.GET("/telegram/qr", server.getTelegramQR)
		api.POST("/telegram/qr/start", server.startTelegramQR)
		api.POST("/telegram/qr/cancel", server.cancelTelegramQR)
	}

	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "story-tts-backend"})
}

func (s *Server) getState(c *gin.Context) {
	state, err := s.manager.AppState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (s *Server) listStories(c *gin.Context) {
	stories, err := s.manager.ListStories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if stories == nil {
		stories = []model.Story{}
	}
	c.JSON(http.StatusOK, gin.H{"items": stories})
}

func (s *Server) scanStory(c *gin.Context) {
	var req struct {
		SourceDir string `json:"sourceDir" binding:"required"`
		Title     string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	detail, err := s.manager.ScanLocalStory(c.Request.Context(), req.SourceDir, req.Title)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (s *Server) importFolder(c *gin.Context) {
	var req model.ImportFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	snapshot, err := s.manager.ImportFolder(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if snapshot.Stories == nil {
		snapshot.Stories = []model.Story{}
	}
	c.JSON(http.StatusOK, snapshot)
}

func (s *Server) getStory(c *gin.Context) {
	storyID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id khong hop le"})
		return
	}
	detail, err := s.manager.GetStoryDetail(c.Request.Context(), storyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (s *Server) getChapterContent(c *gin.Context) {
	chapterID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id khong hop le"})
		return
	}
	item, err := s.manager.GetChapterContent(c.Request.Context(), chapterID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) buildStory(c *gin.Context) {
	storyID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id khong hop le"})
		return
	}
	var req struct {
		Preset model.ProsodyPreset `json:"preset"`
	}
	_ = c.ShouldBindJSON(&req)
	job, err := s.manager.QueueBuildStory(c.Request.Context(), storyID, req.Preset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (s *Server) buildChapter(c *gin.Context) {
	chapterID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id khong hop le"})
		return
	}
	var req struct {
		Preset model.ProsodyPreset `json:"preset"`
	}
	_ = c.ShouldBindJSON(&req)
	job, err := s.manager.QueueBuildChapter(c.Request.Context(), chapterID, req.Preset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (s *Server) listJobs(c *gin.Context) {
	items, err := s.manager.ListJobs(c.Request.Context(), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) getReaderProgress(c *gin.Context) {
	storyID, err := parseID(c.Param("storyId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "story_id khong hop le"})
		return
	}
	item, err := s.manager.GetReaderProgress(c.Request.Context(), storyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) saveReaderProgress(c *gin.Context) {
	var req model.ReaderProgress
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.manager.SaveReaderProgress(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) toggleEdgeReadAloud(c *gin.Context) {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Edge Read Aloud automation hiện chỉ hỗ trợ Windows"})
		return
	}

	script := `
$wshell = New-Object -ComObject WScript.Shell
$null = $wshell.AppActivate('Microsoft Edge')
Start-Sleep -Milliseconds 180
$wshell.SendKeys('^+u')
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "Không gửi được phím tắt Edge Read Aloud",
			"detail": string(output),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) getJob(c *gin.Context) {
	jobID, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id khong hop le"})
		return
	}
	item, err := s.manager.GetJob(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) listBotProfiles(c *gin.Context) {
	items, err := s.manager.ListBotProfiles(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) saveBotProfile(c *gin.Context) {
	var req model.TelegramBotProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.manager.SaveBotProfile(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) sendTelegramCode(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.manager.TelegramSendCode(c.Request.Context(), req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "account": item})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) signInTelegram(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.manager.TelegramSignIn(c.Request.Context(), req.Phone, req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "account": item})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) telegramPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.manager.TelegramPassword(c.Request.Context(), req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "account": item})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) getTelegramQR(c *gin.Context) {
	item := s.manager.CurrentTelegramQRLogin()
	if item == nil {
		c.JSON(http.StatusOK, gin.H{"item": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (s *Server) startTelegramQR(c *gin.Context) {
	item, err := s.manager.StartTelegramQRLogin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "item": item})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) cancelTelegramQR(c *gin.Context) {
	item := s.manager.CancelTelegramQRLogin()
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func parseID(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}
