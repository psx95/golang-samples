package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	cloudprofiler "cloud.google.com/go/cloudprofiler/apiv2"
	pb "cloud.google.com/go/cloudprofiler/apiv2/cloudprofilerpb"
	"google.golang.org/api/iterator"
)

var project = flag.String("project", "", "GCP project ID from which profiles should be fetched")
var pageSize = flag.Int("page_size", 100, "Number of profiles fetched per page. Maximum 1000.")
var pageToken = flag.String("page_token", "", "PageToken from a previous ListProfiles call. If empty, the listing will start from the begnning. Invalid page tokens result in error.")
var maxProfiles = flag.Int("max_profiles", 1000, "Maximum number of profiles to fetch across all pages. If this is <= 0, will fetch all available profiles")

// This function reads profiles for a given project and stores them into locally created files.
// The profile metadata gets stored into a 'metdata.csv' file, while the individual pprof files
// are created per profile.
func run(ctx context.Context) error {
	client, err := cloudprofiler.NewExportClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Printf("Attempting to fetch %v profiles with a pageSize of %v for %v\n", *maxProfiles, *pageSize, *project)

	// Initial request for the ListProfiles API
	request := &pb.ListProfilesRequest{
		Parent:    fmt.Sprintf("projects/%s", *project),
		PageSize:  int32(*pageSize),
		PageToken: *pageToken,
	}

	// create a folder for storing profiles & metadata
	if err := os.Mkdir("profiles", os.ModePerm); err != nil {
		log.Fatal(err)
	}
	// create a file for storing profile metadata
	metadata, err := os.Create("profiles/metadata.csv")
	if err != nil {
		return err
	}
	defer metadata.Close()

	writer := csv.NewWriter(metadata)
	defer writer.Flush()

	writer.Write([]string{"File", "Name", "ProfileType", "Target", "Duration", "Labels"})

	profileCount := 0
	// Keep calling ListProfiles API till all profile pages are fetched or max pages reached
	profilesIterator := client.ListProfiles(ctx, request)
	for {
		// Read individual profile - the client will automatically make API calls to fetch next pages
		profile, err := profilesIterator.Next()

		if err == iterator.Done {
			log.Println("Read all available profiles")
			break
		}
		if err != nil {
			return fmt.Errorf("error reading profile from response: %v", err)
		}
		profileCount++

		filename := fmt.Sprintf("profiles/profile%06d.pprof", profileCount)
		err = os.WriteFile(filename, profile.ProfileBytes, 0)

		if err != nil {
			fmt.Printf("Error writing %s, skipping: %v\n", filename, err)
			continue
		}

		labelBytes, err := json.Marshal(profile.Labels)
		if err != nil {
			return err
		}

		err = writer.Write([]string{filename, profile.Name, profile.Deployment.Target, profile.Duration.String(), string(labelBytes)})
		if err != nil {
			return err
		}

		if *maxProfiles > 0 && profileCount >= *maxProfiles {
			log.Println("Read max allowed profiles")
			break
		}

		if profilesIterator.PageInfo().Remaining() == 0 {
			// This signifies that the client will make a new API call internally
			fmt.Printf("next page token: %v\n", profilesIterator.PageInfo().Token)
		}
	}
	return nil
}

func main() {
	flag.Parse()
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
	log.Println("Finished reading all profiles")
}
