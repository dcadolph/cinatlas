FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /cinatlas .

FROM gcr.io/distroless/static-debian12
COPY --from=build /cinatlas /cinatlas
# The response cache lands here; ephemeral disk is fine for a cache.
ENV XDG_CACHE_HOME=/tmp/cache
EXPOSE 8878
ENTRYPOINT ["/cinatlas", "serve"]
