# Minimal image for the SCANOSS CLI. GoReleaser injects the pre-built static
# binary into the build context, so there is no Go build step here.
FROM gcr.io/distroless/static:nonroot
COPY scanoss /usr/local/bin/scanoss
ENTRYPOINT ["/usr/local/bin/scanoss"]
