package loglevel

// dpa overrides for loglevel need to be 10 or greater to see debug logs.
const debug int = 10
const info int = 0
const UnsupportedOverridesKey = "loglevel"

var verbosity int = info

// Set verbosity of logs. Higher values mean more verbose logs.
func SetLogLevel(v int) {
	verbosity = v
}

func Debug() int {
	return atLeastZero(debug - verbosity)
}

func atLeastZero(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
