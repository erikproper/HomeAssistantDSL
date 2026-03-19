migrate:
	go run .

migrate-vienna:
	go run . migrate Vienna

migrate-junglinster:
	go run . migrate Junglinster

interpret:
	go run . interpret

interpret-vienna:
	go run . interpret Vienna

interpret-junglinster:
	go run . interpret Junglinster

refresh:
	go run . refresh

test-migration:
	go test ./...
