package valid

import "testing"

type embedded struct {
	Amount float64 `validate:"required"`
}

type nested struct {
	ID string `validate:"required"`
}

type withEmbedded struct {
	embedded
	Currency nested
	Items    []nested `validate:"dive"`
}

func TestValidateErrorKeys(t *testing.T) {
	valerr := New().Validate(&withEmbedded{Items: []nested{{}}})
	if valerr == nil {
		t.Fatal("expected validation errors")
	}
	for _, key := range []string{"Amount", "Currency.ID", "Items[0].ID"} {
		if _, ok := valerr.Errors[key]; !ok {
			t.Errorf("missing error key %q, got %v", key, valerr.Errors)
		}
	}
}

type dates struct {
	Start int `validate:"required"`
	Stop  int `validate:"omitempty,gtfield=Start"`
}

func TestGtField(t *testing.T) {
	v := New()
	if err := v.Validate(&dates{Start: 1, Stop: 2}); err != nil {
		t.Errorf("stop after start: unexpected %v", err.Errors)
	}
	if err := v.Validate(&dates{Start: 2, Stop: 1}); err == nil || err.Errors["Stop"].Tag != "gtfield" {
		t.Errorf("stop before start: want gtfield error on Stop, got %v", err)
	}
}
