package connectedpolicy

const dockerEnrollmentUnit = "home-lab-observer-connected-rich-docker-enrollment.service"

func dockerOfflineExpected(e OfflineExpectation) (map[string]string, error) {
	want, err := offlineExpectedForProfile(e, true)
	if err != nil {
		return nil, err
	}
	want["Id"] = dockerEnrollmentUnit
	want["BindReadOnlyPaths"] = "/opt/home-lab-observer-connected-rich/releases/" + e.ArtifactSHA256 + "/observer-connected-rich-docker-uploader:/bin/observer-connected-rich-docker-uploader:rbind"
	want["ExecPaths"] = "/bin/observer-connected-rich-docker-uploader"
	return want, nil
}
func dockerOnlineExpected(e OnlineExpectation) (map[string]string, error) {
	want, err := dockerOfflineExpected(OfflineExpectation{UploaderUID: e.UploaderUID, UploaderGID: e.UploaderGID, SharedGID: e.SharedGID, ArtifactSHA256: e.ArtifactSHA256, Mode: "validate-enrollment"})
	if err != nil {
		return nil, err
	}
	want["PrivateNetwork"] = "no"
	want["RestrictAddressFamilies"] = "AF_INET AF_INET6"
	return want, nil
}
