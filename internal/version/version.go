package version

// Version information set at build time via ldflags:
// go build -ldflags "-X github.com/dustin/Caddystat/internal/version.Version=1.0.0"
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)
