VERSION=$(shell grep "VERSION = " cli/cmd/root.go | cut -d"\"" -f2 | tr -d '\n')
BUILD=$(shell git rev-parse HEAD)
BASEDIR=./dist
DIR=${BASEDIR}/temp

BUILD_UUID=$(shell cat /proc/sys/kernel/random/uuid)

LDFLAGS=-ldflags "-s -w -X 'main.build=${BUILD}' -X 'github.com/archimoebius/fishler/app.ServiceUUIDString=${BUILD_UUID}' -buildid=${BUILD}"
GCFLAGS=-gcflags=all=-trimpath=$(shell pwd)
ASMFLAGS=-asmflags=all=-trimpath=$(shell pwd)

GOFILES=`go list -buildvcs=false ./...`
GOFILESNOTEST=`go list -buildvcs=false ./... | grep -v test`

# Make Directory to store executables
$(shell mkdir -p ${DIR})

all: linux freebsd
# goreleaser build --config .goreleaser.yml --rm-dist --skip-validate

freebsd: lint security docs
	@env CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/fishler-freebsd_amd64 main.go

linux: lint security docs
	@echo "LDFLAGS: ${LDFLAGS}"
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/fishler-linux_amd64 main.go

docs:
	@go run main.go doc
	@mv docs/* mkdocs/docs/usage/

tidy:
	@go mod tidy

update: tidy
	@go get -v -d ./...
	@go get -u all

dep: ## Get the dependencies # go get github.com/goreleaser/goreleaser
	@git config --global url."git@github.com:".insteadOf "https://github.com/"
	@go install github.com/boumenot/gocover-cobertura@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest

lint: ## Lint the files
	@env CGO_ENABLED=0 go fmt ${GOFILES}
	@env CGO_ENABLED=0 go vet ${GOFILESNOTEST}

security: dep tidy
	@go run github.com/securego/gosec/v2/cmd/gosec@latest -quiet ./...
	@go run github.com/go-critic/go-critic/cmd/gocritic@latest check -enableAll -disable='#experimental,#opinionated' ./...
	@go run github.com/google/osv-scanner/cmd/osv-scanner@latest -r . || echo "oh snap!"

release:
	@goreleaser release --config .github/goreleaser.yml

clean:
	@rm -rf ${BASEDIR}
	@rm -rf mkdocs/docs/usage/

.PHONY: all freebsd linux docs submodule tidy update dep lint security test release clean
