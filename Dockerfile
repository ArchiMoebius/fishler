FROM alpine
COPY --chmod=755 bash /usr/bin/
ENTRYPOINT ["bash"]
