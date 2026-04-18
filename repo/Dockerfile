# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS deps
WORKDIR /src

# Dependencies resolve from the committed go.mod + go.sum so Docker
# never runs `go mod tidy` at build time. This prevents any runtime
# install path from sneaking into the image or into test execution.
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/harborclass ./cmd/harborclass

# Dedicated test stage - baked into the image so ./run_tests.sh can run
# the full suite inside Docker without any host toolchain or installs.
FROM deps AS tests
COPY . .
CMD ["go", "test", "-count=1", "./..."]

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/harborclass /app/harborclass
COPY --from=build /src/web /app/web
COPY --from=build /src/internal/store/migrations.sql /app/migrations.sql

EXPOSE 8080
ENV HARBORCLASS_HTTP_ADDR=:8080
ENV HARBORCLASS_DB_URL=postgres://harbor:harbor@db:5432/harborclass?sslmode=disable
ENV HARBORCLASS_MIGRATIONS=/app/migrations.sql

CMD ["/app/harborclass"]
