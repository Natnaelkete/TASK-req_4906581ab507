# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy source
COPY . .

# Resolve and vendor dependencies on the fly. Using -mod=mod lets us
# bootstrap go.sum inside the container so the repo ships lean while
# Docker produces a reproducible image.
ENV GOFLAGS=-mod=mod
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/harborclass ./cmd/harborclass

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
