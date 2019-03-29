package solo

import (
	"errors"
	"log"
	"math/big"
	"strconv"
	"strings"

	"github.com/webchain-network/webchaind/common"
	"gopkg.in/redis.v3"
)

const maxFinders = 100

type BlockFinder struct {
	Login  string
	Height int64
}

func formatKey(prefix string, args ...interface{}) string {
	return join(prefix, join(args...))
}

func join(args ...interface{}) string {
	s := make([]string, len(args))
	for i, v := range args {
		switch v.(type) {
		case string:
			s[i] = v.(string)
		case int64:
			s[i] = strconv.FormatInt(v.(int64), 10)
		case uint64:
			s[i] = strconv.FormatUint(v.(uint64), 10)
		case float64:
			s[i] = strconv.FormatFloat(v.(float64), 'f', 0, 64)
		case bool:
			if v.(bool) {
				s[i] = "1"
			} else {
				s[i] = "0"
			}
		case *big.Int:
			n := v.(*big.Int)
			if n != nil {
				s[i] = n.String()
			} else {
				s[i] = "0"
			}
		default:
			panic("Invalid type specified for conversion")
		}
	}
	return strings.Join(s, ":")
}
func weiToShannonInt64(wei *big.Rat) int64 {
	shannon := new(big.Rat).SetInt(common.Shannon)
	inShannon := new(big.Rat).Quo(wei, shannon)
	value, _ := strconv.ParseInt(inShannon.FloatString(0), 10, 64)
	return value
}
func GetBlockFinder(client *redis.Client, height int64) (*BlockFinder, error) {
	h := strconv.FormatInt(height, 10)
	option := redis.ZRangeByScore{Min: h, Max: h}
	cmd := client.ZRangeByScoreWithScores(formatKey("web", "blocks", "finders"), option)

	if cmd.Err() != nil {
		return nil, cmd.Err()
	}

	if len(cmd.Val()) == 0 {
		return nil, errors.New("No Entries Found For Block Finder")
	}

	entry := cmd.Val()[0]

	finder := BlockFinder{Login: strings.Split(entry.Member.(string), ":")[0], Height: int64(entry.Score)}
	return &finder, nil
}
func PurgeBlockFinders(client *redis.Client, minCount int) int64 {
	option := redis.ZRangeByScore{Min: "-inf", Max: "+inf"}
	cmd := client.ZRangeByScoreWithScores(formatKey("web", "blocks", "finders"), option)
	maxCount := len(cmd.Val())

	if maxCount <= minCount {
		return 0
	}

	numRemoved := client.ZRemRangeByRank(formatKey("web", "blocks", "finders"), 0, (int64)(maxCount-minCount)-1)

	return numRemoved.Val()
}
func CalculateRewards(client *redis.Client, height int64, minersProfit *big.Rat, rewards map[string]int64) map[string]int64 {

	finder, err := GetBlockFinder(client, height)

	if err != nil {
		//no block finder so payout per share instead
		log.Printf("Failed to find a block finder for height %v, falling back to PROP", height)

		return rewards

	} else {
		//Solo payouts
		log.Printf("Found a block finder for height %v, using solo payout policy", height)
		rewards = make(map[string]int64)
		rewards[finder.Login] = weiToShannonInt64(minersProfit)

	}

	numPurged := PurgeBlockFinders(client, maxFinders)
	log.Printf("Purged %v BlockFinders", numPurged)

	return rewards
}
func WriteFinder(client *redis.Client, height uint64, login string, nonce string) (bool, error) {

	log.Println("Writing block finder to backend")

	d := join(login, nonce) //login:nonce
	cmd := client.ZAdd(formatKey("web", "blocks", "finders"), redis.Z{Score: float64(height), Member: d})
	return false, cmd.Err()
}
