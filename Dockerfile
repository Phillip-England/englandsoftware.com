FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY static ./static
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/englandsoftware .

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/englandsoftware /app/englandsoftware
EXPOSE 8493
CMD ["/app/englandsoftware"]
