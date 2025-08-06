sed -e '9s/true/false/' \
-e '19s/true/false/' \
-e 's@"dir": "/data"@"dir": "/data/hot"@' \
-e 's@"/warm-data"@"/data/warm"@' \
-e 's@"send-file-info": true@"send-file-info": false@' \
-e 's@"/remote-data"@"/data/cold"@' \
-e '101s/,//' \
-e '102,103d' \
-e '59,60d' \
-e '30,34d' \
-e '29s/,//' \
./examples/hornet_config.json > ./hornet_config.json
