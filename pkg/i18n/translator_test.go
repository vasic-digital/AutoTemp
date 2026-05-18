// CONST-035 / Article XI §11.9 unit coverage for the AutoTemp i18n
// translator seam. Tests assert observable behaviour (string equality on
// returned keys, behaviour invariance across nil/empty params) rather
// than presence of constructors.
package i18n_test

import (
	"sync"
	"testing"

	"digital.vasic.autotemp/pkg/i18n"
)

func TestNoopTranslator_ReturnsKeyVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}

	cases := []struct {
		key string
	}{
		{"autotemp_run_summary"},
		{"autotemp_advanced_summary"},
		{"autotemp_benchmark_summary"},
		{"autotemp_err_invalid_configuration"},
		{"autotemp_err_invalid_parameters"},
		{"autotemp_err_invalid_dataset_item"},
		{"autotemp_err_runner_failed"},
		{"autotemp_err_judge_failed"},
		{"autotemp_err_baseline_runner_not_configured"},
		{"autotemp_err_prompt_required"},
		{"autotemp_err_model_name_required"},
		{""},
		{"unknown_key_with_dots.and.colons:plus-dashes"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.key, func(t *testing.T) {
			got := tr.T(c.key, nil)
			if got != c.key {
				t.Fatalf("NoopTranslator.T(%q, nil) = %q, want key verbatim", c.key, got)
			}
		})
	}
}

func TestNoopTranslator_IgnoresParams(t *testing.T) {
	tr := i18n.NoopTranslator{}
	key := "autotemp_run_summary"
	params := map[string]any{
		"temperature": 0.7,
		"score":       0.93,
		"candidates":  4,
	}

	got := tr.T(key, params)
	if got != key {
		t.Fatalf("NoopTranslator.T(%q, params) = %q, want %q (params must be ignored)", key, got, key)
	}
}

func TestNoopTranslator_ConcurrentSafe(t *testing.T) {
	tr := i18n.NoopTranslator{}
	const goroutines = 64
	const iterations = 256

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if got := tr.T("autotemp_benchmark_summary", nil); got != "autotemp_benchmark_summary" {
					t.Errorf("concurrent T() returned %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// fakeTranslator is a unit-test-only test double — permitted under
// CONST-050(A) because this file is *_test.go. It proves the Translator
// interface is satisfiable by a non-Noop implementation and gives the
// call-site tests a way to verify that SetTranslator actually rewires
// rendering.
type fakeTranslator struct {
	calls map[string]int
	mu    sync.Mutex
}

func (f *fakeTranslator) T(key string, _ map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[key]++
	return "translated:" + key
}

func TestTranslator_InterfaceSatisfaction(t *testing.T) {
	var _ i18n.Translator = i18n.NoopTranslator{}
	var _ i18n.Translator = &fakeTranslator{}
}
