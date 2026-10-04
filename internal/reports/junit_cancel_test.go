package reports

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseJUnitSecretScanHonorsCancellation(t *testing.T) {
	input := `<testsuite><system-out>` + strings.Repeat("x", 256<<10) + `</system-out></testsuite>`
	secrets := make([]string, 1000000)
	for i := range secrets {
		secrets[i] = "absent-secret-value"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := ParseJUnit(ctx, strings.NewReader(input), secrets)
	if !errors.Is(err, ErrCancelled) {
		t.Fatal("cancelled scan did not stop", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("secret scan continued long after cancellation")
	}
}
