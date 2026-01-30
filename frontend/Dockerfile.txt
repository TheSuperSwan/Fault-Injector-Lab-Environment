FROM registry.access.redhat.com/ubi10/nginx-126:10.0

RUN rm -rf /opt/app-root/src/*

COPY guestbook-frontend/index.html /opt/app-root/src/index.html

COPY guestbook-frontend/nginx.conf /etc/nginx/nginx.conf

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
