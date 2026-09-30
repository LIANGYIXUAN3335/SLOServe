.PHONY: gateway test bench reference dashboard build

gateway:
	cd gateway && go run ./cmd/gateway

test:
	cd gateway && go test ./...
	python3 -m unittest bench.test_run

bench:
	python3 bench/run.py --endpoint http://127.0.0.1:8080 --requests 48 --concurrency 12 --max-tokens 10 --batch-ratio 0.35 --output bench/results/latest.json

reference:
	bash scripts/run-reference.sh

dashboard:
	npm run dev

build:
	npm run build
