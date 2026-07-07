FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.26-openshift-4.23 AS builder
WORKDIR /go/src/github.com/openshift/cert-manager-operator

ARG GO_BUILD_TAGS=strictfipsruntime,openssl
ENV GOEXPERIMENT=strictfipsruntime
ENV CGO_ENABLED=1

COPY . .
RUN go build -mod=vendor -tags $GO_BUILD_TAGS -ldflags '-w -s' \
    -o /app/cert-manager-http01-proxy ./cmd/http01-proxy

FROM registry.access.redhat.com/ubi9-minimal@sha256:062c52ff973065752b0965787649db2bcf551a6c727a00e95a3eb42cebadbdab
COPY --from=builder /app/cert-manager-http01-proxy /usr/local/bin/cert-manager-http01-proxy
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/cert-manager-http01-proxy"]
