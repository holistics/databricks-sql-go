package errors

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// categorizedError is a minimal tagged error used to exercise CategoryFromError.
type categorizedError struct {
	msg      string
	category ErrorCategory
	err      error
}

func (e categorizedError) Error() string           { return e.msg }
func (e categorizedError) Category() ErrorCategory { return e.category }
func (e categorizedError) Unwrap() error           { return e.err }

func withCategory(msg string, cause error, category ErrorCategory) error {
	return categorizedError{msg: msg, category: category, err: cause}
}

func TestCategoryFromError(t *testing.T) {
	t.Run("nil error has no category", func(t *testing.T) {
		assert.Equal(t, ErrorCategory(""), CategoryFromError(nil))
	})

	t.Run("untagged error has no category", func(t *testing.T) {
		err := errors.Wrap(errors.New("cause"), "boom")
		assert.Equal(t, ErrorCategory(""), CategoryFromError(err))
	})

	t.Run("returns the category set at the source", func(t *testing.T) {
		err := withCategory("boom", errors.New("cause"), CategoryChunkDownload)
		assert.Equal(t, CategoryChunkDownload, CategoryFromError(err))
	})

	t.Run("walks the chain to a tagged inner error", func(t *testing.T) {
		// errors.As would stop at the untagged outer wrapper; the walk must not.
		inner := withCategory("inner", errors.New("cause"), CategoryResultSet)
		wrapped := errors.Wrap(inner, "outer")
		assert.Equal(t, CategoryResultSet, CategoryFromError(wrapped))
	})

	t.Run("innermost (source) category wins over an outer tag", func(t *testing.T) {
		// Pins innermost-wins: a future outer-wrapper tag must not mask the source.
		inner := withCategory("inner", errors.New("cause"), CategoryResultSet)
		outer := withCategory("outer", inner, CategoryChunkDownload)
		assert.Equal(t, CategoryResultSet, CategoryFromError(outer))
	})

	t.Run("a generic outer tag does not mask a specific inner one", func(t *testing.T) {
		inner := withCategory("inner", errors.New("cause"), CategoryChunkDownload)
		outer := withCategory("outer", inner, CategoryGeneric)
		assert.Equal(t, CategoryChunkDownload, CategoryFromError(outer))
	})

	t.Run("empty outer category does not shadow a tagged inner", func(t *testing.T) {
		inner := withCategory("inner", errors.New("cause"), CategoryResultSet)
		outer := withCategory("outer", inner, "")
		assert.Equal(t, CategoryResultSet, CategoryFromError(outer))
	})

	t.Run("finds the tag through a fmt.Errorf %w wrapper", func(t *testing.T) {
		inner := withCategory("inner", errors.New("cause"), CategoryResultSet)
		wrapped := fmt.Errorf("outer: %w", inner)
		assert.Equal(t, CategoryResultSet, CategoryFromError(wrapped))
	})

	t.Run("finds the tag inside a tree-shaped chain", func(t *testing.T) {
		inner := withCategory("inner", errors.New("cause"), CategoryResultSet)
		joined := stderrors.Join(errors.New("sibling"), inner)
		assert.Equal(t, CategoryResultSet, CategoryFromError(joined))

		multiWrap := fmt.Errorf("a: %w, b: %w", errors.New("sibling"), inner)
		assert.Equal(t, CategoryResultSet, CategoryFromError(multiWrap))
	})
}

// TestCategoryStringValues pins every category to its exact wire string (a
// dashboard dimension) so a typo fails the build instead of shipping silently.
func TestCategoryStringValues(t *testing.T) {
	cases := []struct {
		category ErrorCategory
		want     string
	}{
		{CategoryTimeout, "timeout"},
		{CategoryCancelled, "cancelled"},
		{CategoryConnectionError, "connection_error"},
		{CategoryAuthError, "auth_error"},
		{CategoryPermissionError, "permission_error"},
		{CategoryNotFound, "not_found"},
		{CategorySyntaxError, "syntax_error"},
		{CategoryInvalidRequest, "invalid_request"},
		{CategoryGeneric, "error"},
		{CategoryChunkDownload, "chunk_download_error"},
		{CategoryDecompression, "decompression_error"},
		{CategoryArrowSchemaParsing, "arrow_schema_parsing_error"},
		{CategoryResultSet, "result_set_error"},
		{CategoryUnsupportedOperation, "unsupported_operation"},
		{CategoryRateLimitExceeded, "rate_limit_exceeded"},
		{CategorySSLHandshake, "ssl_handshake_error"},
		{CategoryStatementTimeout, "statement_execution_timeout"},
		{CategoryExecuteStatement, "execute_statement_failed"},
		{CategoryStatementCancelled, "execute_statement_cancelled"},
		{CategorySessionClosed, "session_closed"},
		{CategoryStatementClosed, "statement_closed"},
	}
	assert.Len(t, cases, 21, "every ErrorCategory constant must be pinned here")
	for _, tc := range cases {
		assert.Equal(t, tc.want, string(tc.category))
	}
}
