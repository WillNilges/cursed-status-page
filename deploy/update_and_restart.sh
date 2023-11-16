#!/bin/bash
set -e
docker pull willnilges/cursed-status-page:main && docker-compose down && docker-compose up -d && echo "Update Complete!"
