FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sloserve ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sloserve /sloserve
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/sloserve"]
