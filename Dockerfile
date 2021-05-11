FROM debian:stable-20210408-slim

ARG VERSION
ARG PG_VERSION=13

ENV PG_VERSION=${PG_VERSION}
ENV PG_HOME=/var/lib/postgresql
ENV PG_DATA=${PG_HOME}/data
ENV PG_LOGS=/var/log/postgresql
ENV PG_BACKUP=/var/backup/postgresql
ENV PG_USER=postgres

# libpq
ENV PGDATA=${PG_DATA}
ENV PGHOST=localhost
ENV PGPORT=5432

LABEL maintainer="vontikov@gmail.com"
LABEL version="${VERSION}"

EXPOSE 5432/tcp
EXPOSE 3501/tcp

RUN \
  DEBIAN_FRONTEND=noninteractive apt-get update -y  \
  && DEBIAN_FRONTEND=noninteractive apt-get dist-upgrade -y \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y wget pwgen sudo lsb-release gnupg \
  && wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add - > /dev/null \
  && echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
  && DEBIAN_FRONTEND=noninteractive apt-get update -y  \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y acl sudo locales \
      postgresql-${PG_VERSION} postgresql-client-${PG_VERSION} postgresql-contrib-${PG_VERSION} \
  && ln -sf ${PG_DATA}/postgresql.conf /etc/postgresql/${PG_VERSION}/main/postgresql.conf \
  && ln -sf ${PG_DATA}/pg_hba.conf /etc/postgresql/${PG_VERSION}/main/pg_hba.conf \
  && ln -sf ${PG_DATA}/pg_ident.conf /etc/postgresql/${PG_VERSION}/main/pg_ident.conf \
  && update-locale LANG=C.UTF-8 LC_MESSAGES=POSIX \
  && locale-gen en_US.UTF-8 \
  && DEBIAN_FRONTEND=noninteractive apt-get remove -y wget \
  && DEBIAN_FRONTEND=noninteractive apt-get autoremove -y \
  && DEBIAN_FRONTEND=noninteractive apt-get clean -y \
  && rm -r /var/lib/apt/lists /var/cache/apt/archives \
  && mkdir -p "${PG_HOME}" \
  && mkdir -p "${PG_LOGS}" \
  && mkdir -p "${PG_BACKUP}" \
  && chown -R ${PG_USER}:${PG_USER} "${PG_HOME}" \
  && chown -R ${PG_USER}:${PG_USER} "${PG_BACKUP}"

COPY assets/docker/scripts/promote /scripts/promote
COPY assets/docker/scripts/sync    /scripts/sync
RUN chmod 755 /scripts/promote /scripts/sync

COPY assets/docker/scripts/functions ${PG_HOME}/functions
COPY assets/docker/scripts/entrypoint /sbin/docker-entrypoint
RUN chmod 755 /sbin/docker-entrypoint
COPY .bin/pgcluster /sbin

USER ${PG_USER}
ENTRYPOINT ["docker-entrypoint"]
CMD ["pgcluster"]
