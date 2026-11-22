# pa-coop : serveur dédié Prison Architect (port TCP unique, 4533 par défaut)
#   docker run -d -p 4533:4533 ghcr.io/fabienmillet/pa-coop
FROM docker.io/library/golang:1.24-alpine AS build
WORKDIR /src
COPY server/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /pacoop-server ./cmd/pacoop-server

FROM scratch
COPY --from=build /pacoop-server /pacoop-server
ENV PACOOP_PORT=4533 PACOOP_MAX_PLAYERS=16
EXPOSE 4533/tcp
USER 65534:65534
ENTRYPOINT ["/pacoop-server"]
