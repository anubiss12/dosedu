package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/repository"
)

type uploadError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

const maxUploadBytes = 5 * 1024 * 1024 // 5 MB, per spec
var allowedExt = map[string]bool{".csv": true, ".xlsx": true}

// requiredColumns are read case-insensitively from the header row —
// matches the "question, option_a, option_b, option_c, option_d,
// answer" format documented in the teacher upload panel.
var requiredColumns = []string{"question", "option_a", "option_b"}

// UploadLevelTest handles POST /teacher/tests/:level/upload?official=true|false
//
// It validates size/format up front, then parses row-by-row, collecting
// per-row errors instead of failing the whole batch (the "error log"
// panel), and — once a file passes cleanly — persists both the upload
// record and every parsed question, so students can actually take the
// test (previously this only validated and never wrote to the DB).
func (d *Deps) UploadLevelTest(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "only teachers may upload level tests"})
		return
	}
	if claims.LanguageScope == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "no_language_scope",
			"message": "Сізге тіл бағыты тағайындалмаған — тест жүктеу қолжетімсіз.",
		})
		return
	}

	level := c.Param("level")
	isOfficial := c.Query("official") == "true"

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	if fileHeader.Size > maxUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "file_too_large",
			"max_bytes":   maxUploadBytes,
			"actual_size": fileHeader.Size,
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedExt[ext] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "unsupported_format",
			"allowed":       []string{".csv", ".xlsx"},
			"received_file": fileHeader.Filename,
		})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read upload"})
		return
	}
	defer file.Close()

	rows, err := readSpreadsheetRows(file, ext)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not parse file", "detail": err.Error()})
		return
	}

	questions, errorLog := parseTestRows(rows)

	status := "ok"
	if len(errorLog) > 0 {
		status = "failed"
	}

	errJSON, err := json.Marshal(errorLog)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode error log"})
		return
	}

	uploadID, err := d.TestUploads.Insert(c.Request.Context(), claims.BranchID, claims.UserID, level, fileHeader.Filename, fileHeader.Size, status, errJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload record"})
		return
	}

	questionsAdded := 0
	if status == "ok" && len(questions) > 0 {
		repoQuestions := make([]repository.NewQuestion, len(questions))
		for i, q := range questions {
			repoQuestions[i] = repository.NewQuestion(q)
		}
		if err := d.Questions.BulkInsert(c.Request.Context(), claims.BranchID, claims.UserID, claims.LanguageScope, level, isOfficial, repoQuestions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save questions"})
			return
		}
		questionsAdded = len(questions)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              uploadID,
		"level":           level,
		"language":        claims.LanguageScope,
		"official":        isOfficial,
		"file_name":       fileHeader.Filename,
		"status":          status,
		"error_log":       errorLog,
		"row_errors":      len(errorLog),
		"questions_added": questionsAdded,
	})
}

// ListTestUploads godoc
// @Summary		List this teacher's recent test uploads
// @Tags			teacher,tests
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/teacher/tests/uploads [get]
func (d *Deps) ListTestUploads(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleTeacher {
		c.JSON(http.StatusForbidden, gin.H{"error": "only teachers have upload history"})
		return
	}
	uploads, err := d.TestUploads.ListByTeacher(c.Request.Context(), claims.UserID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load uploads"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uploads": uploads})
}

// readSpreadsheetRows normalizes .csv and .xlsx into the same [][]string
// shape (including the header row), so the rest of the pipeline doesn't
// care which format was uploaded.
func readSpreadsheetRows(file io.Reader, ext string) ([][]string, error) {
	if ext == ".csv" {
		reader := csv.NewReader(file)
		reader.FieldsPerRecord = -1
		var rows [][]string
		for {
			record, readErr := reader.Read()
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return nil, readErr
			}
			rows = append(rows, record)
		}
		return rows, nil
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}
	return f.GetRows(sheets[0])
}

type parsedQuestion struct {
	Question      string
	OptionA       string
	OptionB       string
	OptionC       string
	OptionD       string
	CorrectOption int
}

// parseTestRows maps the header row to column indexes (case-insensitive,
// order-independent), then validates every data row against the
// question/option_a/option_b/option_c/option_d/answer schema. answer is
// a 1-based index into whichever options are non-empty for that row.
// Errors are collected per row rather than aborting the whole file.
func parseTestRows(rows [][]string) ([]parsedQuestion, []uploadError) {
	var errorLog []uploadError
	if len(rows) == 0 {
		return nil, []uploadError{{Row: 1, Message: "file is empty"}}
	}

	col := make(map[string]int)
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range requiredColumns {
		if _, ok := col[required]; !ok {
			errorLog = append(errorLog, uploadError{Row: 1, Message: fmt.Sprintf(`Missing "%s" column`, required)})
		}
	}
	answerCol, hasAnswerCol := col["answer"]
	if !hasAnswerCol {
		errorLog = append(errorLog, uploadError{Row: 1, Message: `Missing "answer" column`})
	}
	if len(errorLog) > 0 {
		return nil, errorLog
	}

	get := func(record []string, key string) string {
		idx, ok := col[key]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	var questions []parsedQuestion
	for i := 1; i < len(rows); i++ {
		rowNum := i + 1 // 1-based, header is row 1
		record := rows[i]
		if len(record) == 0 || strings.TrimSpace(strings.Join(record, "")) == "" {
			continue // skip blank trailing rows
		}

		question := get(record, "question")
		if question == "" {
			errorLog = append(errorLog, uploadError{Row: rowNum, Message: fmt.Sprintf(`Missing "question" column in row %d`, rowNum)})
			continue
		}
		optA, optB := get(record, "option_a"), get(record, "option_b")
		if optA == "" || optB == "" {
			errorLog = append(errorLog, uploadError{Row: rowNum, Message: fmt.Sprintf("Missing option_a/option_b in row %d", rowNum)})
			continue
		}
		optC, optD := get(record, "option_c"), get(record, "option_d")

		maxIndex := 2
		if optC != "" {
			maxIndex = 3
		}
		if optD != "" {
			maxIndex = 4
		}

		answerRaw := ""
		if answerCol < len(record) {
			answerRaw = strings.TrimSpace(record[answerCol])
		}
		answerIdx, convErr := strconv.Atoi(answerRaw)
		if convErr != nil {
			errorLog = append(errorLog, uploadError{Row: rowNum, Message: fmt.Sprintf("Invalid answer value in row %d", rowNum)})
			continue
		}
		if answerIdx < 1 || answerIdx > maxIndex {
			errorLog = append(errorLog, uploadError{Row: rowNum, Message: fmt.Sprintf("Answer index out of range in row %d", rowNum)})
			continue
		}

		questions = append(questions, parsedQuestion{
			Question:      question,
			OptionA:       optA,
			OptionB:       optB,
			OptionC:       optC,
			OptionD:       optD,
			CorrectOption: answerIdx - 1, // 1-based (spreadsheet-friendly) -> 0-based
		})
	}

	return questions, errorLog
}
