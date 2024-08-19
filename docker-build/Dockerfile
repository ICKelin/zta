FROM ubuntu:22.04
RUN mkdir -p /opt/apps/zta
COPY . /opt/apps/zta
COPY start.sh /
RUN chmod +x start.sh && chmod +x /opt/apps/zta
CMD /start.sh