module github.com/mypersonalfork/gascity

go 1.22

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/cors v1.2.1
	github.com/joho/godotenv v1.5.1
	go.uber.org/zap v1.27.0
)

require (
	go.uber.org/multierr v1.11.0 // indirect
	// note: go.uber.org/atomic is only needed for older zap versions; can remove if upgrading zap past v1.27
	go.uber.org/atomic v1.11.0 // indirect
)

// forked from gastownhall/gascity for personal learning/experimentation
// upstream: https://github.com/gastownhall/gascity
// TODO: go.uber.org/atomic can be dropped once zap is upgraded beyond v1.27.0
// TODO: consider upgrading go.uber.org/zap to v1.28+ to clean up the atomic indirect dep
// personal note: keeping deps conservative for now; revisit when upstream merges chi v5.2
// personal note: checked 2024-06 - upstream still on chi v5.1, no movement on v5.2 yet; will sync again in a month
