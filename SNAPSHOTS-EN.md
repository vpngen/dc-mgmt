# Brigades Encrypted Snapshot

## Required configuration data on the control node

* The control node setup includes the public RSA key of the source data center, which is being snapshotted (stored as a key list `dc-rsa-pub_keys`).
* The control node setup also includes the public keys of the service executors authorized to decrypt the snapshot (`auth-rsa-pub_keys`).
* These keys are distributed in the package `vgkeydesk-snap-auth`.

## Required configuration data in the data center

* The data center setup includes the public RSA key of the source data center, which is being snapshotted (stored as a key list `dc-rsa-pub_keys`).
* The data center setup also includes the public keys of the service executors authorized to decrypt the snapshot (`auth-rsa-pub_keys`).

**TODO**: For restoration, a mechanism with key splitting is used to decrypt the shared secret `psk` for all brigades. The data center has a list of trusted individuals public keys `trusees_keys`.

## Creating a set of brigade snapshots on the control node

* On the control node, a separate SSH entry is required to fetch a set of brigade snapshots (package `vgkeydesk-snap-access`).
* The brigade snapshot collection utility (`/opt/vgkeydesk-snap/fetchsnaps.sh`, package `vgkeydesk-snap`) performs the following steps:
  * Takes the following inputs:
    * A shared secret for encryption (`psk`), unique to this snapshot creation process.
    * Snapshot set tag and the overall start time of the snapshot creation process.
    * End time of the maintenance mode (if required).
    * Fingerprint of the data center's public key where the backup is being performed (requester must know this key).
    * List of brigades to be snapped.
  * Iterates through the list of brigades (skips those not requested) and executes the snapshot collection script for each brigade using sudo.
  * Combines the results into a snapshot set.
  * Adds the values of the snapshot count and snapshot capture error count.
  * Returns the result.
* The utility for collecting a snapshot of a single brigade (`/opt/vgkeydesk-snap/snapshot`, package `vgkeydesk-snap`) performs the following steps:
  * Takes the following inputs:
    * A shared secret for encryption (`psk`), unique to this snapshot creation process.
    * Snapshot set tag and the overall start time of the snapshot creation process.
    * End time of the maintenance mode (if required).
    * Fingerprint of the data center's public key where the backup is being performed (requester must know this key).
  * Generates secrets:
    * Selects one `dc-rsa-pub_key` from the `dc-rsa-pub_keys` list that corresponds to the provided fingerprint of the data center's public RSA key.
    * `locker` - the secret is then encrypted using the selected data center's public key `dc-rsa-pub_key` to associate the final snapshot with the data center (and  later re-encrypted in the data center for a specific service executor).
    * `secret` or `main secret` - the secret is then encrypted using the public keys of each service executor (`auth-rsa-pub_keys`); it restricts the ability to decrypt  to a limited set of service executors at the control node level.
  * Calculates the final encryption key `<tag> + <brigade ID> + <overall snapshot creation start time in Unix time> + <current Unix time> + psk + locker + secret`  (requires applying metadata).
  * Compresses `brigade.json` using the gzip algorithm and encrypts the resulting compressed data similar to `openssl enc -aes-256-cbc -pass zzz`.
  * Enables the maintenance mode for the brigade if the corresponding parameters were provided.

## Collecting a set of brigade snapshots in the data center

* The utility for collecting a set of brigade snapshots in the data center (`/opt/vg-dc-snaps/collectsnaps.sh`, package `dc-mgmt`) takes the following inputs:
  * Snapshot set tag.
  * Brigade filter by external IP address (optional).
  * Brigade filter by control node IP (optional) ??
  * Flag to add the date to the snapshot set file name (optional).
  * Flag to overwrite the snapshot set with the same tag (optional).
* The utility for collecting a set of brigade snapshots in the data center performs the following steps:
  * Generates a shared secret for encryption (`psk`), unique to this snapshot creation process.
  * Selects one `dc-rsa-pub_key` from the `dc-rsa-pub_keys` list that corresponds to the provided fingerprint of the data center's public RSA key.
  * Encrypts `psk` using the selected data center's public RSA key `dc-rsa-pub_key`.
    * **TODO:** In the future, there will likely be a split key, and metadata will be added to the decryption message in the form of a tag, snapshot time, filter  and  data center to allow the executor to control which specific snapshot they were asked to provide their part of the key for. At this stage, I decided not t   overcomplicate it.
    * **TODO:** A special key is used to decrypt `psk`.
      * During the snapshot creation, it is split between trusted individuals, and `tag + snapshot start time + key part` is encrypted using the trusted  individual's  public RSA key `turstee-pub_key`.
      * Before decryption, it is necessary to obtain as many decrypted key parts as required for consensus.
      * For simplicity, the initial consensus is set to 2, and one of the `turstee-pub_keys` is the data center's key.
      * Accordingly, there is a special utility that creates requests for decrypting the `psk` key, and the re-encryption utility accepts these key parts as  an argument  and re-encrypts `psk` with the specified password (to avoid any keys).
  * Determines the overall start time of the snapshot creation process.
  * Takes snapshots of all brigades, including the values of the snapshot count and snapshot capture error count.
  * Adds the instance identifier of the brigade in the data center to each brigade snapshot.
  * If provided, the filter by external IP address is recorded inside the snapshot for information.
  * If provided, the filter by control node IP is recorded inside the snapshot for information???
* The data center periodically takes a set of snapshots of all brigades in the data center with the option to overwrite the snapshot set.
* For migration purposes, a special snapshot set is intended to be created with a filter based on the external IP addresses of the brigades.