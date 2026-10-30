GO ?= go

.PHONY: test ci vet install

# 日常：快速跑全部測試
test:
	$(GO) test ./...

# commit 前：格式、vet、race
ci: vet
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "未 gofmt：$$unformatted"; exit 1; fi
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

install:
	$(GO) install ./cmd/dnd
