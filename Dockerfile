FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY static ./static
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/englandsoftware .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/englandsoftware /app/englandsoftware
COPY articles /app/articles
EXPOSE 8493
CMD ["/app/englandsoftware"]
