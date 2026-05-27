FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ournezt-web ./cmd/web

FROM alpine:3.20

RUN adduser -D -H -u 10001 appuser
WORKDIR /app
COPY --from=build /out/ournezt-web /app/ournezt-web
COPY templates /app/templates
COPY static /app/static
USER appuser

EXPOSE 8080
ENV WEB_ADDR=:8080
ENTRYPOINT ["/app/ournezt-web"]
