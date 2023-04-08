FROM golang:1.19-alpine as builder

COPY . /app
WORKDIR /app

RUN go build -o /app/main .

FROM alpine
COPY --from=builder /app/main /app/main
CMD ["/app/main"]