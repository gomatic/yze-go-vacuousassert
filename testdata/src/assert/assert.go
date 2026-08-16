package assert

// T is testify's minimal surface, reproduced so the fixture needs no module
// outside testdata.
type T interface{ Errorf(format string, args ...any) }

func Equal(t T, expected, actual any, args ...any) bool    { return true }
func ErrorIs(t T, err, target any, args ...any) bool        { return true }
func NotErrorIs(t T, err, target any, args ...any) bool     { return true }
func Same(t T, expected, actual any, args ...any) bool      { return true }
func Contains(t T, haystack, needle any, args ...any) bool  { return true }
func True(t T, value bool, args ...any) bool                { return true }
func NoError(t T, err error, args ...any) bool              { return true }
