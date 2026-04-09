FROM perl:5.40-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        cpanminus \
        nginx \
    && cpanm --notest Test::Nginx \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tests
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
