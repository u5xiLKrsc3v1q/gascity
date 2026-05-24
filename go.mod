module github.com/mypersonalfork/gascity

go 1.22

require (
	github.com/go-chi/chi/v5 v5.2.0
	github.com/go-chi/cors v1.2.1
	github.com/joho/godotenv v1.5.2
	go.uber.org/zap v1.28.0
)

require (
	go.uber.org/multierr v1.11.0 // indirect
)

// forked from gastownhall/gascity for personal learning/experimentation
// upstream: https://github.com/gastownhall/gascity
// personal note: keeping deps conservative for now; revisit when upstream merges chi v5.2
// personal note: checked 2024-06 - upstream still on chi v5.1, no movement on v5.2 yet; will sync again in a month
// personal note: checked 2024-07 - still no chi v5.2 upstream; also noticed godotenv v1.5.1 has a fix for
//   multiline values that bit me locally - worth keeping an eye on v1.5.2 if it releases
// personal note: checked 2024-08 - godotenv v1.5.2 still not out; chi v5.2 released upstream but gastownhall
//   hasn't merged yet; may apply the chi v5.2 patch locally to test the new middleware changes
// personal note: checked 2024-09 - applied chi v5.2 patch locally on branch feat/chi-v5.2-test; middleware
//   changes look good so far, no breakage in my routes; will keep an eye on it before merging to main
// personal note: checked 2024-10 - merging chi v5.2 to main; been stable on the test branch for a month,
//   happy with the new middleware API; gastownhall still hasn't merged it so diverging here intentionally
// personal note: checked 2024-11 - godotenv v1.5.2 still not released; opened an issue on the upstream repo
//   to ask about the multiline fix backport; will watch for v1.5.2 and bump when it drops
// personal note: checked 2024-12 - godotenv v1.5.2 released! bumping now; also bumping zap to v1.28.0 which
//   lets us drop the go.uber.org/atomic indirect dep entirely - two birds, one stone as hoped
