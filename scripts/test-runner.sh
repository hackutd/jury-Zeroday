#!/bin/bash

# This script will run the test using docker compose!
# It will poll for the jury-testing container status until it has exited.
# Then it will read the container logs to see if any tests have failed.
# Logs will be printed to the console.
# Finally, it will exit and kill the docker compose containers

# Max time to wait for the tests to finish (seconds). Includes building/compiling
# the backend and test containers, so keep this generous for CI.
MAX_WAIT=${TEST_TIMEOUT:-300}

# Start docker containers and wait a second (just for fun)
docker compose -f docker-compose.test.yml up -d --build
sleep 1

echo "Running tests..."

timeout=0
while [ "$timeout" -lt "$MAX_WAIT" ]; do
    if [ "$(docker inspect --format '{{.State.Status}}' jury-testing 2>/dev/null)" = "exited" ]; then
        break
    fi
    sleep 1
    timeout=$((timeout + 1))
done

# Get logs and get rid of containers
logs=$(docker logs jury-testing 2>&1)
if [ "$timeout" -ge "$MAX_WAIT" ]; then
    docker compose -f docker-compose.test.yml logs go-dev
fi
docker compose -f docker-compose.test.yml down
printf "%s\n" "$logs"

# Timed out before the test container finished
if [ "$timeout" -ge "$MAX_WAIT" ]; then
    printf "\n\n##### TESTS TIMED OUT AFTER %ss #####\n" "$MAX_WAIT"
    exit 1
fi

# Only pass if the suite reported a full pass and nothing failed (incl. setup failures, logged as FAILED)
if echo "$logs" | grep -q "Entire test suite passed!" && ! echo "$logs" | grep -qi "failed"; then
    printf "\n\nSuccess :)\n"
    exit 0
fi

# Otherwise, exit with failure
printf "\n\n###################################\n##### THERE ARE TEST FAILURES #####\n###################################\n"
exit 1
