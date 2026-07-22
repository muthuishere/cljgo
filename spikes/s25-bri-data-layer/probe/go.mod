module cljgo-spike-s25

go 1.26

require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/lib/pq v1.10.9
	github.com/muthuishere/cljgo v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/muthuishere/cljgo => ../../..
