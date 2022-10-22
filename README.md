# IFPS pinner

# setup

Install IPFS with CLI on the machine ([official doc](https://docs.ipfs.tech/install/command-line/#official-distributions))

Configure your environement:

```
export GITHUB_TOKEN="<get on github>"
export PINNER_API_TOKEN="<generate random>"
export IFPS_DIRECTORY="<where your file will be stored>"
export PINNER_ADDR=":5050"
```

Run the server:

```
./ipfs-pinner
```

Use the HTTP API:

```
curl -v -H 'Authorization: Bearer <secret>' -d'{"filename": "<file URL accessible by the server>"}' http://localhost:5050/
```

Or the websocket client:

```
cat << EOF >
{
  "fileName1": "fileurl1",
  "fileName2": "fileurl2"
}
EOF
./ipfs-pinner-cli -file ./files.json [-addr SERVER_ADDR]
```
