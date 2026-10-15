package project

import (
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Run contains only sanitized evidence. IDs and evidence never change after insertion.
type Run struct {
	ScenarioID     string          `json:"scenarioId,omitempty"`
	Scenario       *ScenarioReport `json:"scenario,omitempty"`
	ScenarioRunID  string          `json:"scenarioRunId,omitempty"`
	StepID         string          `json:"stepId,omitempty"`
	ID             string          `json:"id"`
	CaseID         string          `json:"caseId,omitempty"`
	Environment    string          `json:"environment"`
	StartedAt      time.Time       `json:"startedAt"`
	InputDigest    string          `json:"inputDigest"`
	ContractDigest string          `json:"contractDigest"`
	Outcome        string          `json:"outcome"`
	Error          string          `json:"error,omitempty"`
	Request        Exchange        `json:"request"`
	Response       Exchange        `json:"response"`
	Pinned         bool            `json:"pinned"`
}
type HistoryLimits struct {
	PerCase  int   `json:"perCase" yaml:"perCase"`
	MaxBytes int64 `json:"maxBytes" yaml:"maxBytes"`
}
type HistoryPage struct {
	Items           []Run         `json:"items"`
	Total           int           `json:"total"`
	NextOffset      *int          `json:"nextOffset"`
	PinnedBytes     int64         `json:"pinnedBytes"`
	PinnedOverLimit bool          `json:"pinnedOverLimit"`
	Limits          HistoryLimits `json:"limits"`
}

func historyDB(root string, fn func(*sql.DB) error) error {
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		p, err := localPath(root, s.Manifest.ID)
		if err != nil {
			return err
		}
		dir := filepath.Join(filepath.Dir(filepath.Dir(p)), "history")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		p = filepath.Join(dir, strings.TrimSuffix(filepath.Base(p), ".json")+".sqlite")
		f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		f.Close()
		db, err := sql.Open("sqlite", p)
		if err != nil {
			return err
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		_, err = db.Exec(`PRAGMA secure_delete=ON; PRAGMA auto_vacuum=FULL;
CREATE TABLE IF NOT EXISTS runs (seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE NOT NULL, case_id TEXT NOT NULL, pinned INTEGER NOT NULL DEFAULT 0, evidence BLOB NOT NULL, bytes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS limits (id INTEGER PRIMARY KEY CHECK(id=1), per_case INTEGER NOT NULL, max_bytes INTEGER NOT NULL);
INSERT OR IGNORE INTO limits VALUES (1,100,104857600);`)
		if err != nil {
			return err
		}
		return fn(db)
	})
}
func historyLimits(db *sql.DB) (v HistoryLimits, err error) {
	err = db.QueryRow("SELECT per_case,max_bytes FROM limits WHERE id=1").Scan(&v.PerCase, &v.MaxBytes)
	return
}
func pruneHistory(db *sql.DB) error {
	limits, err := historyLimits(db)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`DELETE FROM runs WHERE pinned=0 AND seq IN (SELECT seq FROM (SELECT seq, ROW_NUMBER() OVER (PARTITION BY case_id ORDER BY seq DESC) AS n FROM runs WHERE pinned=0 AND case_id!='') WHERE n>?)`, limits.PerCase)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM runs WHERE pinned=0 AND seq IN (SELECT seq FROM (SELECT seq,SUM(bytes) OVER (ORDER BY seq DESC) AS retained FROM runs WHERE pinned=0) WHERE retained>?)`, limits.MaxBytes)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (c *Credentials) recordRun(root string, r Run) error {
	for i := range r.Response.Checks {
		r.Response.Checks[i].Message = c.Sanitize(r.Response.Checks[i].Message)
	}

	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return historyDB(root, func(db *sql.DB) error {
		if _, err := db.Exec("INSERT INTO runs(id,case_id,evidence,bytes) VALUES (?,?,?,?)", r.ID, r.CaseID, b, len(b)); err != nil {
			return err
		}
		return pruneHistory(db)
	})
}
func History(root, caseID string, limit, offset int) (page HistoryPage, err error) {
	if limit < 1 || limit > 1000 || offset < 0 {
		return page, Fail("invalid_input", "history limit must be 1..1000 and offset nonnegative")
	}
	page.Items = []Run{}
	err = historyDB(root, func(db *sql.DB) error {
		var e error
		page.Limits, e = historyLimits(db)
		if e != nil {
			return e
		}
		if e = db.QueryRow("SELECT COALESCE(SUM(bytes),0) FROM runs WHERE pinned=1").Scan(&page.PinnedBytes); e != nil {
			return e
		}
		page.PinnedOverLimit = page.PinnedBytes > page.Limits.MaxBytes
		if e = db.QueryRow("SELECT COUNT(*) FROM runs WHERE (?='' OR case_id=?)", caseID, caseID).Scan(&page.Total); e != nil {
			return e
		}
		rows, e := db.Query("SELECT id,case_id,pinned,evidence FROM runs WHERE (?='' OR case_id=?) ORDER BY seq DESC LIMIT ? OFFSET ?", caseID, caseID, limit, offset)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			r, e := scanRun(rows)
			if e != nil {
				return e
			}
			r.Scenario = nil
			r.Request.Body = ""
			r.Request.Headers = nil
			r.Response.Body = ""
			r.Response.Headers = nil
			r.Response.Checks = nil
			page.Items = append(page.Items, PreviewRun(r, 0))
		}
		if offset+len(page.Items) < page.Total {
			n := offset + len(page.Items)
			page.NextOffset = &n
		}
		return rows.Err()
	})
	return
}

type runScanner interface{ Scan(...any) error }

func scanRun(row runScanner) (r Run, err error) {
	var id, caseID string
	var pinned bool
	var b []byte
	err = row.Scan(&id, &caseID, &pinned, &b)
	if err != nil {
		return
	}
	err = json.Unmarshal(b, &r)
	r.ID = id
	r.CaseID = caseID
	r.Response.CaseID = caseID
	r.Pinned = pinned
	return
}
func GetRun(root, id string) (r Run, err error) {
	err = historyDB(root, func(db *sql.DB) error {
		var e error
		r, e = scanRun(db.QueryRow("SELECT id,case_id,pinned,evidence FROM runs WHERE id=?", id))
		if e == sql.ErrNoRows {
			return Fail("not_found", "run not found")
		}
		return e
	})
	return
}
func ChangeRun(root, id, action, caseID string) error {
	return historyDB(root, func(db *sql.DB) error {
		var res sql.Result
		var err error
		switch action {
		case "pin", "unpin":
			res, err = db.Exec("UPDATE runs SET pinned=? WHERE id=?", action == "pin", id)
		case "delete":
			res, err = db.Exec("DELETE FROM runs WHERE id=? AND pinned=0", id)
		case "associate":
			r, e := scanRun(db.QueryRow("SELECT id,case_id,pinned,evidence FROM runs WHERE id=?", id))
			if e != nil {
				return e
			}
			if r.Scenario != nil {
				return Fail("invalid_input", "scenario reports cannot be associated with a case")
			}
			items, e := cases(root)
			if e != nil {
				return e
			}
			c, e := resolveCase(items, caseID)
			if e != nil {
				return e
			}
			res, err = db.Exec("UPDATE runs SET case_id=? WHERE id=? AND case_id=''", c.ID, id)
		default:
			return Fail("invalid_input", "unknown history action")
		}
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return Fail("invalid_input", "run missing, pinned, or already associated; unpin before deletion")
		}
		return pruneHistory(db)
	})
}
func PruneHistory(root string) error { return historyDB(root, pruneHistory) }
func SetHistoryLimits(root string, v HistoryLimits) error {
	if v.PerCase < 0 || v.MaxBytes < 0 {
		return Fail("invalid_input", "history limits must be nonnegative")
	}
	return historyDB(root, func(db *sql.DB) error {
		_, err := db.Exec("UPDATE limits SET per_case=?,max_bytes=? WHERE id=1", v.PerCase, v.MaxBytes)
		if err != nil {
			return err
		}
		return pruneHistory(db)
	})
}

func newRunID() string { return uuid.NewString() }

// PreviewRun bounds text exposed by automated history inspection.
func PreviewRun(r Run, bodyBytes int) Run {

	clip := func(s string, n int) string {
		if len(s) > n {
			return strings.ToValidUTF8(s[:n], "�")
		}
		return s
	}
	if r.Scenario != nil {
		report := *r.Scenario
		if len(report.Steps) > 50 {
			report.Steps = report.Steps[:50]
			report.StepsTruncated = true
		}
		report.Steps = append([]ScenarioStepReport(nil), report.Steps...)
		// Aggregate inspection returns step metadata; exchange bodies are available
		// through the separately paged history records, bounded by PreviewRun.
		for i := range report.Steps {
			step := &report.Steps[i]
			step.ID = clip(step.ID, 128)
			step.Reason = clip(step.Reason, 512)
			if step.Error != nil {
				e := *step.Error
				e.Message = clip(e.Message, 512)
				step.Error = &e
			}
			step.Exchange = Exchange{RunID: step.Exchange.RunID}
		}
		r.Scenario = &report
	}
	r.Environment = clip(r.Environment, 512)
	r.Error = clip(r.Error, 512)
	for _, x := range []*Exchange{&r.Request, &r.Response} {
		x.Method = clip(x.Method, 32)
		x.Destination = clip(x.Destination, 1024)
		if len(x.Body) > bodyBytes {
			x.Body = clip(x.Body, bodyBytes)
			x.Truncated = true
		}
		headers := make(map[string][]string)
		for k, values := range x.Headers {
			if len(headers) == 50 {
				break
			}
			out := []string{}
			for i, v := range values {
				if i == 5 {
					break
				}
				out = append(out, clip(v, 512))
			}
			headers[clip(k, 128)] = out
		}
		x.Headers = headers
		if len(x.Checks) > 50 {
			x.Checks = x.Checks[:50]
		}
		for i := range x.Checks {
			x.Checks[i].Message = clip(x.Checks[i].Message, 512)
		}
	}
	return r
}

func (c *Credentials) historyRequestEvidence(p PreparedRequest, d RequestDraft, used []localCredential) Exchange {
	request := Exchange{Method: c.Sanitize(d.Method), Destination: c.Sanitize(SanitizedDestination(d.URL)), BodyWithheld: true}
	if p.request != nil {
		request.Method = p.Method
		request.Destination = c.Sanitize(SanitizedDestination(p.request.URL.String()))
		request.Headers = p.request.Header.Clone()
		if p.request.GetBody != nil {
			if body, e := p.request.GetBody(); e == nil {
				b, readErr := io.ReadAll(io.LimitReader(body, 1<<20))
				body.Close()
				request.RetainedBytes = len(b)
				request.Truncated = len(b) == 1<<20
				request.Binary = !utf8.Valid(b) || strings.ContainsRune(string(b), 0) || binaryContentType(p.request.Header.Get("Content-Type"))
				if readErr == nil && !request.Binary && !request.Truncated {
					request.Body = string(b)
					request.BodyWithheld = false
				}
			}
		}
	}
	c.sanitizeExchange(&request, used)
	return request
}
