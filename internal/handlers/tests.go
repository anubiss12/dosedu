package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/ratelimit"
	"github.com/dosedu/lms/internal/repository"
)

// requireLanguageCourse rejects any test-related request for a
// CARE_AND_PREP (mad/prodlenka) student — "Ешқандай тест ... МҮЛДЕМ
// ЖОҚ" is absolute, so this is checked before anything else on every
// practice/official-test endpoint.
func (d *Deps) requireLanguageCourse(c *gin.Context, studentID string) (*repository.StudentSummary, bool) {
	summary, err := d.Students.GetSummary(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return nil, false
	}
	if summary.CourseType != "language" {
		c.JSON(http.StatusForbidden, gin.H{"error": "care_and_prep students have no tests"})
		return nil, false
	}
	return summary, true
}

func gradeAnswers(questions []repository.QuestionFull, selectedByID map[string]int) (score int, results []QuizResultItem) {
	for _, q := range questions {
		selected := selectedByID[q.ID]
		isCorrect := selected == q.CorrectOption
		if isCorrect {
			score++
		}
		options := []string{q.OptionA, q.OptionB}
		if q.OptionC != "" {
			options = append(options, q.OptionC)
		}
		if q.OptionD != "" {
			options = append(options, q.OptionD)
		}
		results = append(results, QuizResultItem{
			QuestionID: q.ID,
			Question:   q.Question,
			Options:    options,
			Selected:   selected,
			Correct:    q.CorrectOption,
			IsCorrect:  isCorrect,
		})
	}
	return score, results
}

type QuizResultItem struct {
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	Selected   int      `json:"selected"`
	Correct    int      `json:"correct"`
	IsCorrect  bool     `json:"is_correct"`
}

type SubmitAnswer struct {
	QuestionID string `json:"question_id" binding:"required"`
	Selected   int    `json:"selected"`
}

// --- Placement test (public, unauthenticated, unlimited) ---

// GetPlacementQuestions godoc
// @Summary		20 mixed-level placement-test questions
// @Description	Public, unauthenticated, unlimited use. Deliberately spans every level of the subject — the point is to *find* the level, not confirm one. No correct answers are ever sent.
// @Tags			public,tests
// @Produce		json
// @Param			subject	query		string	true	"english or chinese"
// @Success		200		{object}	map[string]any
// @Router			/public/placement-test/questions [get]
func (d *Deps) GetPlacementQuestions(c *gin.Context) {
	subject := c.Query("subject")
	if subject != "english" && subject != "chinese" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject must be english or chinese"})
		return
	}
	questions, err := d.Questions.ListPlacement(c.Request.Context(), subject, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

type SubmitPlacementRequest struct {
	FullName string         `json:"full_name" binding:"required"`
	Phone    string         `json:"phone" binding:"required"`
	Subject  string         `json:"subject" binding:"required,oneof=english chinese"`
	Answers  []SubmitAnswer `json:"answers" binding:"required,min=1"`
	// BranchID is optional — the lead-form's branch picker on the
	// landing page (GET /public/branches). Left empty, the lead is
	// unassigned until a director/staff member picks it up.
	BranchID string `json:"branch_id"`
}

// SubmitPlacementTest godoc
// @Summary		Grade a placement test and save it as a Lead
// @Description	Public, unauthenticated. Rate-limited by IP (Redis) to deter abuse of an anonymous endpoint. The score becomes the Lead's level_test_result — this is the CRM entry point, not a student account.
// @Tags			public,tests
// @Accept			json
// @Produce		json
// @Param			request	body		SubmitPlacementRequest	true	"Answers + contact details"
// @Success		201		{object}	map[string]any
// @Failure		429		{object}	map[string]string	"too many attempts from this IP"
// @Router			/public/placement-test/submit [post]
func (d *Deps) SubmitPlacementTest(c *gin.Context) {
	if d.RateLimit != nil {
		allowed, err := d.RateLimit.Allow(c.Request.Context(), "placement:"+c.ClientIP(), 5, ratelimit.OneHour)
		if err == nil && !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts — please try again later"})
			return
		}
	}

	var req SubmitPlacementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids := make([]string, len(req.Answers))
	selectedByID := make(map[string]int, len(req.Answers))
	for i, a := range req.Answers {
		ids[i] = a.QuestionID
		selectedByID[a.QuestionID] = a.Selected
	}

	questions, err := d.Questions.GetByIDs(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	score, results := gradeAnswers(questions, selectedByID)
	total := len(questions)

	levelResult := req.Subject + ": " + itoa(score) + "/" + itoa(total)
	leadID, err := d.Leads.Insert(c.Request.Context(), req.BranchID, req.FullName, req.Phone, levelResult, req.Subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save lead"})
		return
	}
	if _, err := d.Questions.RecordPlacement(c.Request.Context(), leadID, req.Subject, score, total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save result"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"lead_id": leadID, "score": score, "total": total, "results": results})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// --- Practice test (student, own level, unlimited) ---

// GetPracticeQuestions godoc
// @Summary		10 random questions from the student's own level
// @Description	Student-only. care_and_prep students get 403 — they have no tests at all.
// @Tags			family,tests
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/practice/questions [get]
func (d *Deps) GetPracticeQuestions(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	// Role is already enforced by middleware.RequireRole(auth.RoleStudent)
	// on this route (router.go).
	summary, ok := d.requireLanguageCourse(c, claims.UserID)
	if !ok {
		return
	}
	if summary.Subject == nil || summary.Level == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "not_assigned", "message": "Сізге әлі деңгей тағайындалмаған."})
		return
	}
	questions, err := d.Questions.ListPractice(c.Request.Context(), *summary.Subject, summary.Level, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions, "subject": *summary.Subject, "level": summary.Level})
}

type SubmitPracticeRequest struct {
	Answers []SubmitAnswer `json:"answers" binding:"required,min=1"`
}

// SubmitPracticeTest godoc
// @Summary		Grade a practice-test attempt (unlimited retakes)
// @Tags			family,tests
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		201	{object}	map[string]any
// @Router			/family/practice/submit [post]
func (d *Deps) SubmitPracticeTest(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	summary, ok := d.requireLanguageCourse(c, claims.UserID)
	if !ok {
		return
	}
	if summary.Subject == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "not_assigned"})
		return
	}

	var req SubmitPracticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids := make([]string, len(req.Answers))
	selectedByID := make(map[string]int, len(req.Answers))
	for i, a := range req.Answers {
		ids[i] = a.QuestionID
		selectedByID[a.QuestionID] = a.Selected
	}
	questions, err := d.Questions.GetByIDs(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	score, results := gradeAnswers(questions, selectedByID)
	total := len(questions)

	id, err := d.Questions.RecordPractice(c.Request.Context(), claims.UserID, *summary.Subject, summary.Level, score, total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save attempt"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "score": score, "total": total, "results": results})
}

// ListPracticeHistory godoc
// @Summary		A student's practice-test history
// @Tags			family,tests
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/practice/history [get]
func (d *Deps) ListPracticeHistory(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	attempts, err := d.Questions.ListPracticeHistory(c.Request.Context(), studentID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"attempts": attempts})
}

// ListQuestionBank godoc
// @Summary		A teacher's own question bank (with answer keys)
// @Description	Teacher-only. Used to pick which questions go into a new official test assignment.
// @Tags			teacher,tests
// @Produce		json
// @Security		BearerAuth
// @Param			level	query		string	true	"e.g. A1, HSK1"
// @Param			pool	query		string	true	"practice or official"
// @Success		200		{object}	map[string]any
// @Router			/teacher/questions [get]
func (d *Deps) ListQuestionBank(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Subject != "english" && claims.Subject != "chinese" {
		c.JSON(http.StatusForbidden, gin.H{"error": "no_subject"})
		return
	}
	level := c.Query("level")
	pool := c.Query("pool")
	if level == "" || (pool != "practice" && pool != "official") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "level required, pool must be practice/official"})
		return
	}
	questions, err := d.Questions.ListBank(c.Request.Context(), claims.BranchID, claims.Subject, level, pool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load question bank"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

// --- Official test (teacher assigns to a group, 1 attempt each) ---

type CreateTestAssignmentRequest struct {
	QuestionIDs []string `json:"question_ids" binding:"required,min=1"`
}

// CreateTestAssignment godoc
// @Summary		Assign a one-time official test to a group
// @Description	Teacher-only, and only for their own subject. Every student in the group then gets exactly one attempt (enforced by a DB unique constraint).
// @Tags			teacher,tests
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			groupId	path		string							true	"Group ID"
// @Param			request	body		CreateTestAssignmentRequest	true	"Question bank IDs for this test"
// @Success		201		{object}	map[string]any
// @Router			/teacher/groups/{groupId}/test-assignments [post]
func (d *Deps) CreateTestAssignment(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Subject != "english" && claims.Subject != "chinese" {
		c.JSON(http.StatusForbidden, gin.H{"error": "no_subject", "message": "Сізге тіл пәні тағайындалмаған."})
		return
	}

	groupID := c.Param("groupId")
	group, err := d.Schedule.GetGroup(c.Request.Context(), groupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	if group.Subject != claims.Subject {
		c.JSON(http.StatusForbidden, gin.H{"error": "this group is not your subject"})
		return
	}

	var req CreateTestAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := d.TestAssignments.Create(c.Request.Context(), claims.BranchID, groupID, claims.UserID, group.Subject, group.Level, req.QuestionIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create test assignment"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ListActiveTestAssignments godoc
// @Summary		Official tests waiting for this student (not yet attempted)
// @Tags			family,tests
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/test-assignments/active [get]
func (d *Deps) ListActiveTestAssignments(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if _, ok := d.requireLanguageCourse(c, studentID); !ok {
		return
	}
	assignments, err := d.TestAssignments.ListActiveForStudent(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load assignments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assignments": assignments})
}

// GetTestAssignmentQuestions godoc
// @Summary		Question set for one active official test assignment
// @Tags			family,tests
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string	true	"Assignment ID"
// @Success		200	{object}	map[string]any
// @Router			/family/test-assignments/{id}/questions [get]
func (d *Deps) GetTestAssignmentQuestions(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if _, ok := d.requireLanguageCourse(c, claims.UserID); !ok {
		return
	}
	assignment, err := d.TestAssignments.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	full, err := d.Questions.GetByIDs(c.Request.Context(), assignment.QuestionIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	questions := make([]repository.Question, len(full))
	for i, q := range full {
		questions[i] = repository.Question{ID: q.ID, Question: q.Question, OptionA: q.OptionA, OptionB: q.OptionB, OptionC: q.OptionC, OptionD: q.OptionD}
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions, "subject": assignment.Subject, "level": assignment.Level})
}

// SubmitTestAssignment godoc
// @Summary		Submit an official test attempt (one-time only)
// @Description	409 if this student already has a recorded attempt for this assignment — enforced by the DB's unique index, not just app logic.
// @Tags			family,tests
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		string					true	"Assignment ID"
// @Param			request	body		SubmitPracticeRequest	true	"Answers"
// @Success		201		{object}	map[string]any
// @Failure		409		{object}	map[string]string
// @Router			/family/test-assignments/{id}/submit [post]
func (d *Deps) SubmitTestAssignment(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if _, ok := d.requireLanguageCourse(c, claims.UserID); !ok {
		return
	}

	assignmentID := c.Param("id")
	assignment, err := d.TestAssignments.GetByID(c.Request.Context(), assignmentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}

	var req SubmitPracticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ids := make([]string, len(req.Answers))
	selectedByID := make(map[string]int, len(req.Answers))
	for i, a := range req.Answers {
		ids[i] = a.QuestionID
		selectedByID[a.QuestionID] = a.Selected
	}
	questions, err := d.Questions.GetByIDs(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	score, results := gradeAnswers(questions, selectedByID)
	total := len(questions)

	id, err := d.TestAssignments.RecordOfficialAttempt(c.Request.Context(), claims.UserID, assignmentID, assignment.Subject, assignment.Level, score, total)
	if err != nil {
		if err == repository.ErrAlreadyAttempted {
			c.JSON(http.StatusConflict, gin.H{"error": "already_taken", "message": "Бұл тест бір рет қана тапсырылады — сіз бұрын тапсырғансыз."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save attempt"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "score": score, "total": total, "results": results})
}

// --- AI: explain one missed question, in the student's chosen language ---

type ExplainMistakeRequest struct {
	Question        string `json:"question" binding:"required"`
	ChosenAnswer    string `json:"chosen_answer" binding:"required"`
	CorrectAnswer   string `json:"correct_answer" binding:"required"`
	Subject         string `json:"subject" binding:"required,oneof=english chinese"`      // subject being learned
	ExplainLanguage string `json:"explain_language" binding:"required,oneof=kk ru en zh"` // language the student wants the explanation in
}

// ExplainQuizMistake godoc
// @Summary		AI explanation for one missed question
// @Description	Student-only. Explains in explain_language (the student's choice), independent of the subject being learned.
// @Tags			family,tests,ai
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]string
// @Router			/family/tests/explain [post]
func (d *Deps) ExplainQuizMistake(c *gin.Context) {
	var req ExplainMistakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	explanation, err := d.AI.ExplainMistake(c.Request.Context(), req.Question, req.ChosenAnswer, req.CorrectAnswer, req.Subject, req.ExplainLanguage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get AI explanation"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"explanation": explanation})
}
