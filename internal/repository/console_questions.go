package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

func consoleQuestionOf(q queries.ConsoleQuestion) models.ConsoleQuestion {
	return models.ConsoleQuestion{ID: q.ID, Question: q.Question, SQL: q.Sql, Explain: q.Explain, Chart: q.Chart, CreatedAt: q.CreatedAt}
}

// FindConsoleQuestion is the question asked before with the same text (any
// case), moved to the top of the history; nil when it wasn't.
func (r *Repository) FindConsoleQuestion(ctx context.Context, question string) (*models.ConsoleQuestion, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetConsoleQuestionByText(ctx, question)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := q.TouchConsoleQuestion(ctx, row.ID); err != nil {
		return nil, err
	}
	cq := consoleQuestionOf(row)
	return &cq, nil
}

// SaveConsoleQuestion keeps a question with its SQL in the history (asked
// again, it takes the new SQL), the history cut to models.ConsoleHistoryLen.
func (r *Repository) SaveConsoleQuestion(ctx context.Context, cq models.ConsoleQuestion) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if err := q.SaveConsoleQuestion(ctx, queries.SaveConsoleQuestionParams{
		ID: idgen.New(), Question: cq.Question, Sql: cq.SQL, Explain: cq.Explain, Chart: cq.Chart,
	}); err != nil {
		return err
	}
	return q.PruneConsoleQuestions(ctx, models.ConsoleHistoryLen)
}

// GetConsoleQuestions are the questions asked, latest first.
func (r *Repository) GetConsoleQuestions(ctx context.Context) ([]models.ConsoleQuestion, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListConsoleQuestions(ctx, models.ConsoleHistoryLen)
	if err != nil {
		return nil, err
	}
	out := make([]models.ConsoleQuestion, len(rows))
	for i, row := range rows {
		out[i] = consoleQuestionOf(row)
	}
	return out, nil
}

// DeleteConsoleQuestion drops a question from the history.
func (r *Repository) DeleteConsoleQuestion(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteConsoleQuestion(ctx, id)
}
