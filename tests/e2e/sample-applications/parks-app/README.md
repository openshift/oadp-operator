# Parks App

A simple app to visualize locations of popular Historic Sites and National Parks.

## Installation

Login to your OpenShift cluster.

To deploy:

```bash
./deploy.sh
```

To remove:

```bash
./destroy.sh
```

## Usage

After successful installation, the app should be exposed at a public URL. Wait for 4-5 minutes for all the services to start.

To get the url to the frontend:

```bash
oc get route -n parks-app restify -o go-template='{{ .spec.host }}{{ println }}'
```

## Development

`manifests` directory contains definitions for different resources required by the app. `manifest.yaml` is just a collection of all those definitions.

You may edit individual definitions for different app resources. To combine all YAMLs and create an updated `manifest.yaml`:

```bash
./build.sh
```

## Storage Components

The application uses one PersistentVolumeClaim:

- **`mongodb-data-claim`** (10Gi) - Stores MongoDB database data

Velero backup/restore preserves the container image with all static files already built in, ensuring the web interface works correctly after restore.

## Velero Backup/Restore

For information about Velero backup and restore capabilities, see the [velero/README.md](velero/README.md) documentation.

If you read everything carefully, here's a cookie for you 🍪
