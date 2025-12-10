FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates make git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux make build

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/pingpong /pingpong

EXPOSE 8080

ENTRYPOINT ["/pingpong"]

