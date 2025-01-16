# Use Home Assistant base image
ARG BUILD_FROM
FROM $BUILD_FROM

# Set up tempio (if needed)
ARG TEMPIO_VERSION BUILD_ARCH
RUN curl -sSLf -o /usr/bin/tempio \
    "https://github.com/home-assistant/tempio/releases/download/${TEMPIO_VERSION}/tempio_${BUILD_ARCH}" && \
    chmod +x /usr/bin/tempio

# Copy root filesystem
COPY rootfs /

# Ensure scripts in services.d and cont-init.d are executable
RUN chmod +x /etc/cont-init.d/* /etc/services.d/dd/run /etc/services.d/dd/finish
RUN chmod +x /usr/bin/dd/action /usr/bin/dd/haus /usr/bin/dd/register

# Set s6-overlay environment variables
ENV S6_BEHAVIOUR_IF_STAGE2_FAILS=2
ENV S6_CMD_WAIT_FOR_SERVICES=1

# Set the s6-overlay entrypoint
ENTRYPOINT ["/init"]
CMD []
