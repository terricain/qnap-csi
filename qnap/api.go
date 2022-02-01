package qnap

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type Client struct {
	client                      *http.Client
	baseURL                     *url.URL
	loginEndpoint               string
	diskManageEndpoint          string
	iscsiPortalEndpoint         string
	iscsiTargetSettingsEndpoint string
	iscsiLunSettingsEndpoint    string
	Username                    string
	Password                    string

	sid      string
	sidMutex *sync.RWMutex
}

func NewClient(username, password, qnapURL string) (*Client, error) {
	trimmedBase := strings.TrimRight(qnapURL, "/")
	parsedURL, err := url.Parse(trimmedBase)
	if err != nil {
		return &Client{}, err
	}

	c := Client{
		client:                      &http.Client{},
		baseURL:                     parsedURL,
		loginEndpoint:               trimmedBase + "/cgi-bin/authLogin.cgi",
		diskManageEndpoint:          trimmedBase + "/cgi-bin/disk/disk_manage.cgi",
		iscsiPortalEndpoint:         trimmedBase + "/cgi-bin/disk/iscsi_portal_setting.cgi",
		iscsiTargetSettingsEndpoint: trimmedBase + "/cgi-bin/disk/iscsi_target_setting.cgi",
		iscsiLunSettingsEndpoint:    trimmedBase + "/cgi-bin/disk/iscsi_lun_setting.cgi",
		Username:                    username,
		// Password is sent to the server base64'd
		Password: base64.StdEncoding.EncodeToString([]byte(password)),

		sidMutex: &sync.RWMutex{},
	}

	return &c, nil
}

func addParamsToURL(baseURL string, params url.Values) string {
	parsedURL, _ := url.Parse(baseURL)
	parsedURL.RawQuery = params.Encode()
	return parsedURL.String()
}

func (c *Client) postFormReq(endpoint string, payload string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, strings.NewReader(payload)) // URL-encoded payload
	if err != nil {
		return nil, 0, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(payload)))

	res, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, 0, err
	}

	return body, res.StatusCode, nil
}

func (c *Client) getSid() string {
	c.sidMutex.RLock()
	defer c.sidMutex.RUnlock()
	return c.sid
}

type loginRespXML struct {
	AuthPassed string `xml:"authPassed"`
	AuthSid    string `xml:"authSid"`
}

func (c *Client) Login() error {
	data := url.Values{}
	data.Add("user", c.Username)
	data.Add("pwd", c.Password)

	xmlBytes, statusCode, err := c.postFormReq(c.loginEndpoint, data.Encode())
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return errors.New("status code not 200")
	}

	var xmlStruct loginRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return err
	}

	if xmlStruct.AuthPassed != "1" {
		return errors.New("invalid username or password")
	}

	c.sidMutex.Lock()
	defer c.sidMutex.Unlock()

	c.sid = xmlStruct.AuthSid

	return nil
}

type StoragePoolSubscriptionInfoXML struct {
	PoolID                  int    `xml:"poolID"`
	CapacityBytes           uint64 `xml:"capacity_bytes"`
	FreesizeBytes           uint64 `xml:"freesize_bytes"`
	MaxThickCreateSizeBytes uint64 `xml:"max_thick_create_size_bytes"`
	ThinVolumeTotal         uint64 `xml:"thinVolTotal"`
	ThinLUNTotal            uint64 `xml:"thinLUNTotal"`
	ThickVolumeTotal        uint64 `xml:"thickVolTotal"`
	ThickLUNTotal           uint64 `xml:"thickLUNTotal"`
	VaultTotal              uint64 `xml:"vaultTotal"`
	SnapshotBytes           uint64 `xml:"snapshot_bytes"`
}

type StoragePoolSubscriptionRespXML struct {
	Result           string                         `xml:"result"`
	PoolSubscription StoragePoolSubscriptionInfoXML `xml:"PoolSubscription"`
}

func (c *Client) GetStoragePoolSubscription(poolID int) (StoragePoolSubscriptionRespXML, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("store", "poolSubsc")
	endpoint := addParamsToURL(c.diskManageEndpoint, params)

	data := url.Values{}
	data.Add("func", "extra_get")
	data.Add("Pool_Subs", "1") // the 1 here means nothing
	data.Add("poolID", strconv.Itoa(poolID))

	xmlBytes, statusCode, err := c.postFormReq(endpoint, data.Encode())
	if err != nil {
		return StoragePoolSubscriptionRespXML{}, err
	}
	if statusCode != 200 {
		return StoragePoolSubscriptionRespXML{}, errors.New("status code not 200")
	}

	var xmlStruct StoragePoolSubscriptionRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return StoragePoolSubscriptionRespXML{}, err
	}

	if xmlStruct.Result == "-1" {
		return StoragePoolSubscriptionRespXML{}, errors.New("invalid pool id")
	} else if xmlStruct.Result != "0" {
		return StoragePoolSubscriptionRespXML{}, errors.New("unknown error occurred")
	}

	return xmlStruct, nil
}

type LogicalVolumeInfoXML struct {
	Index                string `xml:"vol_no"`
	Status               string `xml:"vol_status"`
	Label                string `xml:"vol_label"`
	LUNIndex             string `xml:"LUNIndex"`
	Type                 string `xml:"volume_type"`
	StaticVolume         string `xml:"static_volume"`
	StoragePoolID        string `xml:"poolID"`
	VirtualJBOD          string `xml:"pool_vjbod"`
	UIType               string `xml:"volume_ui_type"`
	LogicalVolumeType    string `xml:"lv_type"`
	MetadataSize         string `xml:"tp_metadata_size"`
	EncryptFSEnabled     string `xml:"encryptfs_bool"`
	EncryptFSActive      string `xml:"encryptfs_active_bool"`
	CacheVolIsConverting string `xml:"cache_vol_is_converting"`
	Unclean              string `xml:"unclean"`
	NeedsRehash          string `xml:"need_rehash"`
	Creating             string `xml:"creating"`
	BaseID               string `xml:"baseID"`
	MappingName          string `xml:"mappingName"`
}

func (l *LogicalVolumeInfoXML) VolumeTypeString() string {
	switch l.Type {
	case "1":
		return "Thick Volume"
	case "3":
		return "Block-based Thick LUN"
	default:
		return fmt.Sprintf("Unknown ID %s", l.Type)
	}
}

type StorageLogicalVolumeRespXML struct {
	Result                string                 `xml:"result"`
	VolumeCreatingProcess int                    `xml:"vol_creating_process"`
	Volumes               []LogicalVolumeInfoXML `xml:"Volume_Index>row"`
}

func (c *Client) GetStorageLogicalVolumes() (StorageLogicalVolumeRespXML, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("store", "lvList")
	endpoint := addParamsToURL(c.diskManageEndpoint, params)

	data := url.Values{}
	data.Add("func", "extra_get")
	data.Add("extra_vol_index", "1") // the 1 here means nothing

	xmlBytes, statusCode, err := c.postFormReq(endpoint, data.Encode())
	if err != nil {
		return StorageLogicalVolumeRespXML{}, err
	}
	if statusCode != 200 {
		return StorageLogicalVolumeRespXML{}, errors.New("status code not 200")
	}

	var xmlStruct StorageLogicalVolumeRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return StorageLogicalVolumeRespXML{}, err
	}

	if xmlStruct.Result != "0" {
		return StorageLogicalVolumeRespXML{}, errors.New("unknown error occurred")
	}

	return xmlStruct, nil
}

type StorageISCSILUNTargetXML struct {
	TargetIndex string `xml:"targetIndex"`
	LUNNumber   string `xml:"LUNNumber"`
	LUNEnable   string `xml:"LUNEnable"`
}

type StorageISCSILUNInitiatorXML struct {
	InitiatorIndex string `xml:"initiatorIndex"`
	InitiatorIQN   string `xml:"initiatorIQN"`
	AccessMode     string `xml:"accessMode"` // TODO check if 1 == clustered
}

type StorageISCSILUNRespXML struct {
	AuthPassed             string                        `xml:"authPassed"`
	Result                 string                        `xml:"result"`
	Index                  string                        `xml:"LUNInfo>row>LUNIndex"`
	Name                   string                        `xml:"LUNInfo>row>LUNName"`
	Path                   string                        `xml:"LUNInfo>row>LUNPath"`
	VolFree                string                        `xml:"LUNInfo>row>LUNVolFree"`
	Capacity               string                        `xml:"LUNInfo>row>LUNCapacity"`
	Status                 string                        `xml:"LUNInfo>row>LUNStatus"`
	OPPercent              string                        `xml:"LUNInfo>row>LUNOPPercent"`
	Enable                 string                        `xml:"LUNInfo>row>LUNEnable"`
	ThinAllocate           string                        `xml:"LUNInfo>row>LUNThinAllocate"`
	IsRemoving             string                        `xml:"LUNInfo>row>isRemoving"`
	BMap                   string                        `xml:"LUNInfo>row>bMap"`
	SerialNum              string                        `xml:"LUNInfo>row>LUNSerialNum"`
	BackupStatus           string                        `xml:"LUNInfo>row>LUNBackupStatus"`
	IsSnap                 string                        `xml:"LUNInfo>row>isSnap"`
	CapacityBytes          string                        `xml:"LUNInfo>row>capacity_bytes"`
	VolumeBase             string                        `xml:"LUNInfo>row>VolumeBase"`
	VirtualBased           string                        `xml:"LUNInfo>row>virtual_based"`
	VirtualDiskName        string                        `xml:"LUNInfo>row>virtual_disk_name"`
	WCEnable               string                        `xml:"LUNInfo>row>WCEnable"`
	FUAEnable              string                        `xml:"LUNInfo>row>FUAEnable"`
	Threshold              string                        `xml:"LUNInfo>row>LUNThreshold"`
	NAA                    string                        `xml:"LUNInfo>row>LUNNAA"`
	SectorSize             string                        `xml:"LUNInfo>row>LUNSectorSize"`
	SSDCache               string                        `xml:"LUNInfo>row>ssd_cache"`
	StoragePoolID          string                        `xml:"LUNInfo>row>poolID"`
	VolumeNumber           string                        `xml:"LUNInfo>row>volno"`
	PoolType               string                        `xml:"LUNInfo>row>pool_type"`
	SnapshotCount          string                        `xml:"LUNInfo>row>snapshot_count"`
	LogicalVolumeCapacity  string                        `xml:"LUNInfo>row>lv_capacity"`
	LogicalVolumeAllocated string                        `xml:"LUNInfo>row>lv_allocated"`
	BlockBaseUsedPercent   string                        `xml:"LUNInfo>row>block_base_used_percent"`
	BlockSize              string                        `xml:"LUNInfo>row>block_size"`
	FileBaseUsedPercent    string                        `xml:"LUNInfo>row>file_base_used_percent"`
	Targets                []StorageISCSILUNTargetXML    `xml:"LUNInfo>row>LUNTargetList>row"`
	Initiators             []StorageISCSILUNInitiatorXML `xml:"LUNInfo>row>LUNInitList>LUNInitInfo"`
}

func (l *StorageISCSILUNRespXML) StatusString() string {
	switch l.Status {
	case "-1":
		return "removing"
	case "-2":
		return "not_found"
	case "0":
		return "creating"
	case "1":
		return "ready"
	default:
		return fmt.Sprintf("unknown LUN status %s", l.Status)

	}
}

func (c *Client) GetStorageISCSILun(lunID int) (StorageISCSILUNRespXML, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("func", "extra_get")
	params.Add("lun_info", "1")
	params.Add("lunID", strconv.Itoa(lunID))
	endpoint := addParamsToURL(c.iscsiPortalEndpoint, params)

	xmlBytes, statusCode, err := c.postFormReq(endpoint, "")
	if err != nil {
		return StorageISCSILUNRespXML{}, err
	}
	if statusCode != 200 {
		return StorageISCSILUNRespXML{}, errors.New("status code not 200")
	}

	var xmlStruct StorageISCSILUNRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return StorageISCSILUNRespXML{}, err
	}

	if xmlStruct.Result != "0" {
		return StorageISCSILUNRespXML{}, errors.New("unknown error occurred")
	}

	// The Capacity field seems to have a random newline in it :/
	xmlStruct.Capacity = strings.Trim(xmlStruct.Capacity, "\n")

	return xmlStruct, nil
}

type StorageISCSITargetInitConnInfoXML struct {
	ConnectionType   string `xml:"connection_type"`
	InitiatorIQN     string `xml:"initiatorIQN"`
	IP               string `xml:"IP"`
	ConnectionStatus string `xml:"connection_status"`
	ServerName       string `xml:"server_name"`
}

type StorageISCSITargetInfoXML struct {
	TargetIndex          int                                 `xml:"targetIndex"`
	Name                 string                              `xml:"targetName"`
	IQN                  string                              `xml:"targetIQN"`
	Alias                string                              `xml:"targetAlias"`
	Status               string                              `xml:"targetStatus"`
	TargetLUNs           []int                               `xml:"targetLUNList>LUNIndex"`
	InitiatorConnections []StorageISCSITargetInitConnInfoXML `xml:"initiatorConnList>initiatorConnInfo"`
}

func (t *StorageISCSITargetInfoXML) StatusString() string {
	switch t.Status {
	case "-1":
		return "offline"
	case "0":
		return "ready"
	case "1":
		return "connected"
	default:
		return fmt.Sprintf("unknown status %s", t.Status)
	}
}

type StorageISCSITargetListRespXML struct {
	AuthPassed string                      `xml:"authPassed"`
	Result     string                      `xml:"result"`
	Targets    []StorageISCSITargetInfoXML `xml:"iSCSITargetList>targetInfo"`
}

func (c *Client) GetStorageISCSITargetList() (StorageISCSITargetListRespXML, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("func", "extra_get")
	params.Add("targetList", "1")
	endpoint := addParamsToURL(c.iscsiPortalEndpoint, params)

	// Is a get when using the UI but I have a feeling it doesnt care, it munges get and post parameters
	xmlBytes, statusCode, err := c.postFormReq(endpoint, "")
	if err != nil {
		return StorageISCSITargetListRespXML{}, err
	}
	if statusCode != 200 {
		return StorageISCSITargetListRespXML{}, errors.New("status code not 200")
	}

	var xmlStruct StorageISCSITargetListRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return StorageISCSITargetListRespXML{}, err
	}

	if xmlStruct.Result != "0" {
		return StorageISCSITargetListRespXML{}, errors.New("unknown error occurred")
	}

	return xmlStruct, nil
}

type StorageISCSICreateTargetRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"`
}

// CreateStorageISCSITarget TODO(docs) This is sorta idempotent, you can create the same name multiple times.
func (c *Client) CreateStorageISCSITarget(name string, dataDigest, headerDigest, clusterMode bool) (int, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	endpoint := addParamsToURL(c.iscsiTargetSettingsEndpoint, params)

	data := url.Values{}
	data.Add("func", "add_target")
	data.Add("targetName", name)
	data.Add("targetAlias", name)
	data.Add("bTargetDataDigest", b2is(dataDigest))
	data.Add("bTargetHeaderDigest", b2is(headerDigest))
	data.Add("bTargetClusterEnable", b2is(clusterMode))

	// Is a get when using the UI but I have a feeling it doesnt care, it munges get and post parameters
	xmlBytes, statusCode, err := c.postFormReq(endpoint, data.Encode())
	if err != nil {
		return 0, err
	}
	if statusCode != 200 {
		return 0, errors.New("status code not 200")
	}

	var xmlStruct StorageISCSICreateTargetRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return 0, err
	}

	if xmlStruct.Result < 0 {
		return 0, errors.New("unknown error occurred")
	}

	return xmlStruct.Result, nil
}

type StorageISCSICreateInitiatorRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"`
}

// CreateStorageISCSIInitiator TODO(docs) This is sorta idempotent, you can create multiple times, also doesnt seem to care if you
// give it bogus index id's.
func (c *Client) CreateStorageISCSIInitiator(targetIndex int, chapEnable bool, chapUser, chapPass string, mutualChapEnable bool, mutualChapUser, mutualChapPass string) error {
	params := url.Values{}
	params.Add("sid", c.getSid())
	endpoint := addParamsToURL(c.iscsiTargetSettingsEndpoint, params)

	data := url.Values{}
	data.Add("func", "add_init")
	data.Add("targetIndex", strconv.Itoa(targetIndex))
	data.Add("initiatorIndex", "0")
	data.Add("bCHAPEnable", b2is(chapEnable))
	data.Add("CHAPUserName", chapUser)
	data.Add("CHAPPasswd", chapPass)
	data.Add("bMutualCHAPEnable", b2is(mutualChapEnable))
	data.Add("mutualCHAPUserName", mutualChapUser)
	data.Add("mutualCHAPPasswd", mutualChapPass)

	// Is a get when using the UI but I have a feeling it doesnt care, it munges get and post parameters
	xmlBytes, statusCode, err := c.postFormReq(endpoint, data.Encode())
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return errors.New("status code not 200")
	}

	var xmlStruct StorageISCSICreateInitiatorRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return err
	}

	if xmlStruct.Result < 0 {
		return errors.New("unknown error occurred")
	}

	return nil
}

type StorageISCSIDeleteTargetRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"`
}

func (c *Client) DeleteStorageISCSITarget(targetIndex int) error {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("func", "remove_target")
	params.Add("targetIndex", strconv.Itoa(targetIndex))
	endpoint := addParamsToURL(c.iscsiTargetSettingsEndpoint, params)

	// Is a get when using the UI but I have a feeling it doesnt care, it munges get and post parameters
	xmlBytes, statusCode, err := c.postFormReq(endpoint, "")
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return errors.New("status code not 200")
	}

	var xmlStruct StorageISCSIDeleteTargetRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return err
	}

	if xmlStruct.Result != targetIndex {
		return errors.New("unknown error occurred")
	}

	return nil
}

type StorageISCSICreateBlockLUNRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"` // This is the LUN index
	VolumeID   int    `xml:"volumeID"`
}

// CreateStorageISCSIBlockLUN TODO(docs) !!if you try and create same name it'll cause a crash and the ui will show some errors :D.
func (c *Client) CreateStorageISCSIBlockLUN(name string, storagePoolID int, capacity int, thinAllocate bool, sectorSize int, wcEnable, fuaEnable, ssdCache, enableTiering bool) (StorageISCSICreateBlockLUNRespXML, error) {
	params := url.Values{}
	params.Add("sid", c.getSid())
	endpoint := addParamsToURL(c.iscsiLunSettingsEndpoint, params)

	data := url.Values{}
	data.Add("func", "add_lun")

	data.Add("LUNThinAllocate", b2is(thinAllocate))
	data.Add("LUNName", name)
	data.Add("LUNCapacity", strconv.Itoa(capacity))
	data.Add("LUNSectorSize", strconv.Itoa(sectorSize))

	data.Add("WCEnable", b2is(wcEnable))
	data.Add("FUAEnable", b2is(fuaEnable))
	data.Add("FileIO", "no") // File or block based lun
	data.Add("poolID", strconv.Itoa(storagePoolID))
	data.Add("lv_ifssd", b2yn(ssdCache))
	data.Add("LUNPath", name)
	data.Add("enable_tiering", b2is(enableTiering))

	// Is a get when using the UI but I have a feeling it doesnt care, it munges get and post parameters
	xmlBytes, statusCode, err := c.postFormReq(endpoint, data.Encode())
	if err != nil {
		return StorageISCSICreateBlockLUNRespXML{}, err
	}
	if statusCode != 200 {
		return StorageISCSICreateBlockLUNRespXML{}, errors.New("status code not 200")
	}

	var xmlStruct StorageISCSICreateBlockLUNRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return StorageISCSICreateBlockLUNRespXML{}, err
	}

	if xmlStruct.Result < 0 {
		return StorageISCSICreateBlockLUNRespXML{}, errors.New("unknown error occurred")
	}

	return xmlStruct, nil
}

type StorageISCSIDeleteBlockLUNRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"`
}

// DeleteStorageISCSIBlockLUN TODO(docs) can delete random non existent indexes.
func (c *Client) DeleteStorageISCSIBlockLUN(targetIndex int, runInBackground bool) error {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("func", "remove_lun")
	if runInBackground {
		params.Add("run_background", "1")
	}
	params.Add("LUNIndex", strconv.Itoa(targetIndex))
	endpoint := addParamsToURL(c.iscsiLunSettingsEndpoint, params)

	xmlBytes, statusCode, err := c.postFormReq(endpoint, "")
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return errors.New("status code not 200")
	}

	var xmlStruct StorageISCSIDeleteBlockLUNRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return err
	}

	if xmlStruct.Result != 0 {
		return errors.New("unknown error occurred")
	}

	return nil
}

type StorageISCSIAttachTargetLUNRespXML struct {
	AuthPassed string `xml:"authPassed"`
	Result     int    `xml:"result"` // This is the LUN index
}

// AttachStorageISCSITargetLUN TODO(docs).
func (c *Client) AttachStorageISCSITargetLUN(lunIndex, targetIndex int) error {
	params := url.Values{}
	params.Add("sid", c.getSid())
	params.Add("func", "add_lun")
	params.Add("LUNIndex", strconv.Itoa(lunIndex))
	params.Add("targetIndex", strconv.Itoa(targetIndex))
	endpoint := addParamsToURL(c.iscsiTargetSettingsEndpoint, params)

	xmlBytes, statusCode, err := c.postFormReq(endpoint, "")
	if err != nil {
		return err
	}
	if statusCode != 200 {
		return errors.New("status code not 200")
	}

	var xmlStruct StorageISCSIAttachTargetLUNRespXML
	if err = xml.Unmarshal(xmlBytes, &xmlStruct); err != nil {
		return err
	}

	if xmlStruct.Result != 0 {
		return errors.New("unknown error occurred")
	}

	return nil
}
