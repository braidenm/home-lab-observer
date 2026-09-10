FROM scratch
COPY --chown=0:0 package /package
COPY --chown=0:0 probe /probe
COPY --chown=0:0 release-manifest.json /release-manifest.json
COPY --chown=0:0 archive.json /archive.json
USER 65532:65532
ENTRYPOINT ["/probe"]
