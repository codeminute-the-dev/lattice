package rpcclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMatchErrStr checks that `matchErrStr` can correctly replace the dashes
// with spaces and turn title cases into lowercases for a given error and match
// it against the specified string pattern.
func TestMatchErrStr(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		compatErr error
		matchStr  string
		matched   bool
	}{
		{
			name:      "error without dashes",
			compatErr: errors.New("missing input"),
			matchStr:  "missing input",
			matched:   true,
		},
		{
			name:      "match str without dashes",
			compatErr: errors.New("missing-input"),
			matchStr:  "missing input",
			matched:   true,
		},
		{
			name:      "error with dashes",
			compatErr: errors.New("missing-input"),
			matchStr:  "missing input",
			matched:   true,
		},
		{
			name:      "match str with dashes",
			compatErr: errors.New("missing-input"),
			matchStr:  "missing-input",
			matched:   true,
		},
		{
			name:      "error with title case and dash",
			compatErr: errors.New("Missing-Input"),
			matchStr:  "missing input",
			matched:   true,
		},
		{
			name:      "match str with title case and dash",
			compatErr: errors.New("missing-input"),
			matchStr:  "Missing-Input",
			matched:   true,
		},
		{
			name:      "unmatched error",
			compatErr: errors.New("missing input"),
			matchStr:  "missingorspent",
			matched:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matched := matchErrStr(tc.compatErr, tc.matchStr)
			require.Equal(t, tc.matched, matched)
		})
	}
}

// TestMapRPCErr checks that `MapRPCErr` can correctly map a given error to
// the corresponding error in the `LatticedErrMap` or `compatErrors` map.
func TestMapRPCErr(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Get all known compatible fork errors.
	compatErrors := make([]error, 0, errSentinel)
	for i := uint32(0); i < uint32(errSentinel); i++ {
		err := CompatRPCErr(i)
		compatErrors = append(compatErrors, err)
	}

	// An unknown error should be mapped to ErrUndefined.
	errUnknown := errors.New("unknown error")
	err := MapRPCErr(errUnknown)
	require.ErrorIs(err, ErrUndefined)

	// A known error should be mapped to the corresponding error in the
	// `LatticedErrMap` or `compatErrors` map.
	for latticedErrStr, mappedErr := range LatticedErrMap {
		err := MapRPCErr(errors.New(latticedErrStr))
		require.ErrorIs(err, mappedErr)

		err = MapRPCErr(mappedErr)
		require.ErrorIs(err, mappedErr)
	}

	for _, compatErr := range compatErrors {
		err = MapRPCErr(compatErr)
		require.ErrorIs(err, compatErr)
	}
}

// TestCompatErrorSentinel checks that all defined CompatRPCErr errors are
// added to the method `Error`.
func TestCompatErrorSentinel(t *testing.T) {
	t.Parallel()

	rt := require.New(t)

	for i := uint32(0); i < uint32(errSentinel); i++ {
		err := CompatRPCErr(i)
		rt.NotEqualf(err.Error(), "unknown error", "error code %d is "+
			"not defined, make sure to update it inside the Error "+
			"method", i)
	}
}
