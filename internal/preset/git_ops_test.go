package preset

// Test helpers that build expected command strings for git operations.

func gitInitBareCmd(dir string) string {
	return "git init --bare " + dir
}

func gitFetchCmd(dir, url, ref string) string {
	return "git -C " + dir + " fetch --depth 1 " + url + " " + ref
}

func gitRevParseCmd(dir, ref string) string {
	return "git -C " + dir + " rev-parse " + ref
}

func gitCatFileCmd(dir, hash string) string {
	return "git -C " + dir + " cat-file -t " + hash
}

func gitShowCmd(dir, commit, path string) string {
	return "git -C " + dir + " show " + commit + ":" + path
}

func gitLsTreeCmd(dir, ref string) string {
	return "git -C " + dir + " ls-tree --name-only " + ref
}
