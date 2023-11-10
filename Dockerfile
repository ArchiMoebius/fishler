FROM alpine
COPY --chmod=755 bash /usr/bin/
COPY --chmod=755 fixme /
ENTRYPOINT ["bash"]
