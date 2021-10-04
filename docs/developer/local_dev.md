<h1 align="center">Local Development Enviroment</h1>

### Test your operator changes locally by building your own image

1. Build the image:

    ```
    podman build . -t quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG>
    ```

    *Note:* The above command for `podman build` is to be executed from the root directory of the operator.

2. Push the image to your registry:

    ```
    podman push <IMAGE_ID> quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG>
    ```

    *Note:* <IMAGE_ID> can be found out by running `podman images` after the image has been built.
    <IMAGE_TAG> can be any tag that you would would like to assign to the image. If the `oadp-operator` 
    repository exists in `quay.io/<CONTAINER_REGISTRY_USERNAME>`, then verify if the visibility is set to `public`. 
    If not, change the visibility of the `oadp-operator` repository to `public`. 
    
3. Deploy OADP operator using your image:

    ```
    IMG=quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG> make deploy
    ```
