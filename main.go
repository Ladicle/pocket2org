package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"

	"github.com/motemen/go-pocket/api"
	"github.com/motemen/go-pocket/auth"
)

var configDir string

func init() {
	usr, err := user.Current()
	if err != nil {
		log.Fatalln(err)
	}

	configDir = filepath.Join(usr.HomeDir, ".config", "pocket")
	err = os.MkdirAll(configDir, 0777)
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	consumerKey := getConsumerKey()
	accessToken, err := restoreAccessToken(consumerKey)
	if err != nil {
		log.Fatalln(err)
	}

	client := api.NewClient(consumerKey, accessToken.AccessToken)
	if err := printAndArchive(client); err != nil {
		log.Fatalln(err)
	}
}

func printAndArchive(client *api.Client) error {
	res, err := client.Retrieve(&api.RetrieveOption{})
	if err != nil {
		return err
	}

	buf := bufio.NewWriter(os.Stdout)
	defer buf.Flush()
	for _, item := range res.List {
		if _, err := buf.WriteString(
			fmt.Sprintf("** TODO [[%s][%s]]\n", item.URL(), item.Title())); err != nil {
			return err
		}
		// Archive printed item
		action := api.NewArchiveAction(item.ItemID)
		if _, err := client.Modify(action); err != nil {
			return err
		}
	}
	return nil
}

func getConsumerKey() string {
	consumerKeyPath := filepath.Join(configDir, "consumer_key")
	consumerKey, err := ioutil.ReadFile(consumerKeyPath)

	if err != nil {
		log.Printf("Can't get consumer key: %v", err)
		log.Print("Enter your consumer key (from here https://getpocket.com/developer/apps/): ")

		consumerKey, _, err = bufio.NewReader(os.Stdin).ReadLine()
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(consumerKeyPath, consumerKey, 0600)
		if err != nil {
			panic(err)
		}

		return string(consumerKey)
	}

	return string(bytes.SplitN(consumerKey, []byte("\n"), 2)[0])
}

func restoreAccessToken(consumerKey string) (*auth.Authorization, error) {
	accessToken := &auth.Authorization{}
	authFile := filepath.Join(configDir, "auth.json")

	err := loadJSONFromFile(authFile, accessToken)

	if err != nil {
		log.Println(err)

		accessToken, err = obtainAccessToken(consumerKey)
		if err != nil {
			return nil, err
		}

		err = saveJSONToFile(authFile, accessToken)
		if err != nil {
			return nil, err
		}
	}

	return accessToken, nil
}

func obtainAccessToken(consumerKey string) (*auth.Authorization, error) {
	ch := make(chan struct{})
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/favicon.ico" {
				http.Error(w, "Not Found", 404)
				return
			}

			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintln(w, "Authorized.")
			ch <- struct{}{}
		}))
	defer ts.Close()

	redirectURL := ts.URL

	requestToken, err := auth.ObtainRequestToken(consumerKey, redirectURL)
	if err != nil {
		return nil, err
	}

	url := auth.GenerateAuthorizationURL(requestToken, redirectURL)
	fmt.Println(url)

	<-ch

	return auth.ObtainAccessToken(consumerKey, requestToken)
}

func saveJSONToFile(path string, v interface{}) error {
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	return json.NewEncoder(w).Encode(v)
}

func loadJSONFromFile(path string, v interface{}) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	return json.NewDecoder(r).Decode(v)
}
