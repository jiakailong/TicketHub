FROM golang:1.25-bookworm AS build

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./app/${SERVICE}/cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
ARG SERVICE
COPY --from=build /out/server /app/server
COPY --chmod=0644 --from=build /src/app/${SERVICE}/configs/config.yaml /app/config.yaml

USER nonroot:nonroot
ENTRYPOINT ["/app/server", "-conf", "/app/config.yaml"]

