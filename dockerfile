FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o azdoexporter .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /src/azdoexporter .
EXPOSE 8080
ENTRYPOINT ["./azdoexporter"]