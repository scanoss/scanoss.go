# Minimal image for the SCANOSS CLI. GoReleaser injects the pre-built static
# binaries into the build context under <os>/<arch>/, so there is no Go build
# step here. TARGETOS/TARGETARCH are provided automatically by buildx.
FROM gcr.io/distroless/static:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/scanoss-cli /usr/local/bin/scanoss-cli
ENTRYPOINT ["/usr/local/bin/scanoss-cli"]
