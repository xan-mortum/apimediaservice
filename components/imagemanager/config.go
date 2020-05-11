package imagemanager

type Config struct {
	TmpDir string
}

func NewConfig(tmpDir string) Config {
	return Config{
		TmpDir: tmpDir,
	}
}
