FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tkb-api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tkb-seed ./cmd/seed \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tkb ./cmd/tkb \
 && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /data
COPY --from=build /out/ /usr/local/bin/
COPY --from=build --chown=65532:65532 /data /data
ENV ADDR=:8080 DB_PATH=/data/tkb.db
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/tkb-api"]
