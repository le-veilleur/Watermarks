# ---- Build ----
FROM golang:1.26.0-alpine3.23 AS build

ARG SERVICE_DIR
ARG CMD_PATH

WORKDIR /usr/src/${SERVICE_DIR}

COPY ${SERVICE_DIR}/go.mod ${SERVICE_DIR}/go.sum ./
RUN go mod download
COPY ${SERVICE_DIR}/ .

# -o ${CMD_PATH} = chemin de sortie du binaire, . = package courant
RUN CGO_ENABLED=0 go build -o ${CMD_PATH} .

# ---- Prod (stage final) ----
FROM scratch AS prod

# ARG doit être redéclaré dans chaque stage pour être accessible
ARG CMD_PATH

COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build ${CMD_PATH} /usr/local/bin/service

CMD ["/usr/local/bin/service"]
