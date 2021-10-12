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
    <IMAGE_TAG> can be any tag that you would would like to assign to the image. Ensure that the registry 
    which is hosting the container image is publicly available such that the image can be pulled 
    from your OpenShift cluster. 
    
3. Deploy OADP operator using your image:

    ```
    IMG=quay.io/<CONTAINER_REGISTRY_USERNAME>/oadp-operator:<IMAGE_TAG> make deploy
    ```
