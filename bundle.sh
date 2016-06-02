#!/usr/bin/env bash

git archive --format zip --output ../ae-$(date "+%Y%m%d%H%M")-$(git rev-parse HEAD | cut -b1-6).zip HEAD