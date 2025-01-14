# Use the Home Assistant base image
ARG BUILD_FROM
FROM $BUILD_FROM

# Set up tempio
ARG TEMPIO_VERSION BUILD_ARCH
RUN \
    curl -sSLf -o /usr/bin/tempio \
    "https://github.com/home-assistant/tempio/releases/download/${TEMPIO_VERSION}/tempio_${BUILD_ARCH}" && \
    chmod +x /usr/bin/tempio

# Set environment variables for s6-overlay
ENV S6_BEHAVIOUR_IF_STAGE2_FAILS=2
ENV S6_CMD_WAIT_FOR_SERVICES=1

# Copy the root filesystem
COPY rootfs /

# Ensure scripts have execution permissions
RUN chmod +x /etc/cont-init.d/* /etc/services.d/*/run || true

# Define entrypoint for s6-overlay
ENTRYPOINT ["/init"]
CMD []
