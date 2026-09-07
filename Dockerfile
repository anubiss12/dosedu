# dosedu.kz backend — multi-stage build
FROM golang:1.25-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod ./
COPY . .
# `go mod tidy` also resolves TEST-only transitive dependencies of every
# dependency in the graph (needed for MVS correctness on pre-1.17 style
# modules), which can pull in packages requiring a newer Go toolchain
# than this image ships — even though our own code never touches them.
# `go mod download` + `go build` only resolve what's actually imported
# by our source, and go.mod already lists every direct + indirect
# requirement, so this builds go.sum correctly without that detour.
ENV GOFLAGS=-mod=mod
RUN go mod download

# Generate docs/ from the @-annotation comments on handlers (main.go
# blank-imports this package for gin-swagger). Must run before `go
# build` since the generated package doesn't exist in source control.
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.3
RUN swag init -g cmd/api/main.go -o ./docs

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/dosedu-api ./cmd/api

# ---- final minimal image ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/dosedu-api ./dosedu-api
COPY internal/db/migrations ./migrations

EXPOSE 8080
ENTRYPOINT ["./dosedu-api"]
