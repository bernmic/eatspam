Contains docker related stuff

## build docker images for amd64, armv7 and arm64
```
docker buildx build --push --platform linux/arm/v7,linux/arm64/v8,linux/amd64 --tag darthbermel/eatspam:latest .
```

## encrypt password

```
docker run --rm -it -v "<local config path>:/app/config" darthbermel/eatspam:latest /app/main --encrypt <password>
```