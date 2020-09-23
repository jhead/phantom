package db

import "io/ioutil"

type FilePersistence struct {
	FilePath string
}

func (file FilePersistence) ReadData() (string, error) {
	if contents, err := ioutil.ReadFile(file.FilePath); err != nil {
		return "", err
	} else {
		return string(contents), nil
	}
}

func (file FilePersistence) StoreData(data string) error {
	if err := ioutil.WriteFile(file.FilePath, []byte(data), 0644); err != nil {
		return err
	}

	return nil
}
