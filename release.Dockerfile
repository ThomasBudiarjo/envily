# Image built by GoReleaser from its prebuilt binaries (see .goreleaser.yaml).
# For local builds from source use ./Dockerfile.
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/envily /usr/local/bin/envily
COPY --chown=65532:65532 deploy/empty /data
ENV ENVILY_ADDR=:8080 \
	ENVILY_DB_DSN=/data/envily.db
VOLUME ["/data"]
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD ["/usr/local/bin/envily", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/envily"]
CMD ["server"]
