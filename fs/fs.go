package cfs

import (
	"bazil.org/fuse"
	"bufio"
	"bytes"
	"github.com/ipdcode/containerfs/logger"
	dp "github.com/ipdcode/containerfs/proto/dp"
	mp "github.com/ipdcode/containerfs/proto/mp"
	vp "github.com/ipdcode/containerfs/proto/vp"
	"github.com/ipdcode/containerfs/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"io"
	"os"
	"strconv"
	"sync"
	"syscall"
)

var VolMgrAddr string
var MetaNodeAddr string

const (
	O_RDONLY = os.O_RDONLY // 0    00000000000000000000000000000000
	O_WRONLY = os.O_WRONLY // 1    00000000000000000000000000000001
	O_RDWR   = os.O_RDWR   // 2    00000000000000000000000000000010
	O_APPEND = os.O_APPEND // 1024 00000000000000000000010000000000
	O_CREATE = os.O_CREATE // 64   00000000000000000000000001000000
	O_TRUNC  = os.O_TRUNC  // 512  00000000000000000000001000000000
	O_EXCL   = os.O_EXCL   // 0x4000

	O_MVOPT  = O_RDONLY | 0x20000
	O_TAROPT = O_MVOPT | syscall.O_NONBLOCK
	O_SCPOPT = O_RDONLY | syscall.O_NONBLOCK
)

const (
	chunkSize  = 64 * 1024 * 1024
	bufferSize = 256 * 1024
)

type CFS struct {
	VolID  string
	Status int // 0 ok , 1 readonly 2 invaild
}

func CreateVol(name string, capacity string) int32 {
	conn, err := grpc.Dial(VolMgrAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("CreateVol failed,Dial to volmgr fail :%v\n", err)
		return -1

	}
	defer conn.Close()
	vc := vp.NewVolMgrClient(conn)
	spaceQuota, _ := strconv.Atoi(capacity)
	pCreateVolReq := &vp.CreateVolReq{
		VolName:    name,
		SpaceQuota: int32(spaceQuota),
		MetaDomain: MetaNodeAddr,
	}
	pCreateVolAck, err2 := vc.CreateVol(context.Background(), pCreateVolReq)
	if err2 != nil {
		return -1
	}

	// send to metadata to registry a new map
	conn2, err3 := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err3 != nil {
		logger.Error("CreateVol failed,Dial to metanode fail :%v\n", err2)
		return -1
	}
	defer conn2.Close()
	mc := mp.NewMetaNodeClient(conn2)
	pmCreateNameSpaceReq := &mp.CreateNameSpaceReq{
		VolID: pCreateVolAck.UUID,
	}
	pmCreateNameSpaceAck, err4 := mc.CreateNameSpace(context.Background(), pmCreateNameSpaceReq)
	if err4 != nil {
		return -1
	}
	if pmCreateNameSpaceAck.Ret != 0 {
		logger.Error("CreateNameSpace failed :%v\n", pmCreateNameSpaceAck.Ret)
		return -1
	}

	return 0
}

func GetVolInfo(name string) (int32, *vp.GetVolInfoAck) {

	conn, err := grpc.Dial(VolMgrAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("GetVolInfo failed,Dial to volmgr fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	vc := vp.NewVolMgrClient(conn)
	pGetVolInfoReq := &vp.GetVolInfoReq{
		UUID: name,
	}
	pGetVolInfoAck, err2 := vc.GetVolInfo(context.Background(), pGetVolInfoReq)
	if err2 != nil {
		return 1, nil
	}
	if pGetVolInfoAck.Ret != 0 {
		return 1, nil
	}
	return 0, pGetVolInfoAck
}

func GetFSInfo(name string) (int32, *mp.GetFSInfoAck) {

	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("GetFSInfo failed,Dial to metanode fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pGetFSInfoReq := &mp.GetFSInfoReq{
		VolID: name,
	}
	pGetFSInfoAck, err2 := mc.GetFSInfo(context.Background(), pGetFSInfoReq)
	if err2 != nil {
		return 1, nil
	}
	if pGetFSInfoAck.Ret != 0 {
		return 1, nil
	}
	return 0, pGetFSInfoAck
}

func OpenFileSystem(UUID string) *CFS {
	cfs := CFS{VolID: UUID, Status: 0}
	return &cfs
}

func (cfs *CFS) CreateDir(path string) int32 {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("CreateDir failed,Dial to metanode fail :%v\n", err)
		return -1
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pCreateDirReq := &mp.CreateDirReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	pCreateDirAck, err2 := mc.CreateDir(context.Background(), pCreateDirReq)
	if err2 != nil {
		return -1
	}

	return pCreateDirAck.Ret

}
func (cfs *CFS) Stat(path string) (int32, *mp.InodeInfo) {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("Stat failed,Dial to metanode fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pStatReq := &mp.StatReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	pStatAck, err2 := mc.Stat(context.Background(), pStatReq)
	if err2 != nil {
		return -1, nil
	}

	return pStatAck.Ret, pStatAck.InodeInfo

}
func (cfs *CFS) List(path string) (int32, []*mp.InodeInfo) {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("List failed,Dial to metanode fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pListReq := &mp.ListReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	pListAck, err2 := mc.List(context.Background(), pListReq)
	if err2 != nil {
		return -1, nil
	}

	return pListAck.Ret, pListAck.InodeInfos

}
func (cfs *CFS) DeleteDir(path string) int32 {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("DeleteDir failed,Dial to metanode fail :%v\n", err)
		return -1
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pDeleteDirReq := &mp.DeleteDirReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	pDeleteDirAck, err2 := mc.DeleteDir(context.Background(), pDeleteDirReq)
	if err2 != nil {
		return -1
	}
	return pDeleteDirAck.Ret
}

func (cfs *CFS) Rename(path1 string, path2 string) int32 {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("Rename failed,Dial to metanode fail :%v\n", err)
		return -1
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pRenameReq := &mp.RenameReq{
		FullPathName1: path1,
		FullPathName2: path2,
		VolID:         cfs.VolID,
	}
	pRenameAck, err2 := mc.Rename(context.Background(), pRenameReq)
	if err2 != nil {
		return -1
	}

	return pRenameAck.Ret
}

func (cfs *CFS) CreateFile(path string, flags int) (int32, *CFile) {

	if flags&O_TRUNC != 0 {
		if ret, _ := cfs.Stat(path); ret == 0 {
			cfs.DeleteFile(path)
		}
	}

	if flags&O_EXCL != 0 {
		if ret, _ := cfs.Stat(path); ret == 0 {
			return 17, nil
		}
	}

	cfile := CFile{}
	if ret := cfs.createFile(path); ret != 0 {
		return ret, nil
	}

	tmpBuffer := wBuffer{
		buffer:   new(bytes.Buffer),
		freeSize: bufferSize,
	}

	wChannel := make(chan *wBuffer, 128)
	cfile = CFile{
		Path:      path,
		OpenFlag:  flags,
		cfs:       cfs,
		FileSize:  0,
		ReaderMap: make(map[fuse.HandleID]*ReaderInfo),

		wChannel: wChannel,
		wBuffer:  tmpBuffer,
	}
	go cfile.flushChannel()

	return 0, &cfile
}

func (cfs *CFS) OpenFile(path string, flags int) (int32, *CFile) {
	var ret int32
	var writer int32 = 0
	var tmpFileSize int64 = 0

	cfile := CFile{}

	if (flags&O_WRONLY) != 0 || (flags&O_RDWR) != 0 {

		if (flags & O_APPEND) != 0 {
			chunkInfos := make([]*mp.ChunkInfo, 0)

			if ret, chunkInfos = cfs.GetFileChunks(path); ret != 0 {
				return ret, nil
			}

			for i := range chunkInfos {
				tmpFileSize += int64(chunkInfos[i].ChunkSize)
			}
			lastChunk := chunkInfos[len(chunkInfos)-1]

			tmpBuffer := wBuffer{
				buffer:    new(bytes.Buffer),
				freeSize:  bufferSize - (lastChunk.ChunkSize % bufferSize),
				chunkInfo: lastChunk,
			}

			wChannel := make(chan *wBuffer, 128)
			cfile = CFile{
				Path:      path,
				OpenFlag:  flags,
				cfs:       cfs,
				Writer:    writer,
				FileSize:  tmpFileSize,
				wChannel:  wChannel,
				wBuffer:   tmpBuffer,
				chunks:    chunkInfos,
				ReaderMap: make(map[fuse.HandleID]*ReaderInfo),
			}
			go cfile.flushChannel()
		} else {

			cfs.DeleteFile(path)
			if ret = cfs.createFile(path); ret != 0 {
				return ret, nil
			}

			tmpBuffer := wBuffer{
				buffer:   new(bytes.Buffer),
				freeSize: bufferSize,
			}

			wChannel := make(chan *wBuffer, 128)
			cfile = CFile{
				Path:      path,
				OpenFlag:  flags,
				cfs:       cfs,
				Writer:    writer,
				FileSize:  0,
				wChannel:  wChannel,
				wBuffer:   tmpBuffer,
				chunks:    nil,
				ReaderMap: make(map[fuse.HandleID]*ReaderInfo),
			}
			go cfile.flushChannel()
		}
	} else {
		chunkInfos := make([]*mp.ChunkInfo, 0)
		if ret, chunkInfos = cfs.GetFileChunks(path); ret != 0 {
			return ret, nil
		}
		for i := range chunkInfos {
			tmpFileSize += int64(chunkInfos[i].ChunkSize)
		}

		tmpBuffer := wBuffer{
			buffer:   new(bytes.Buffer),
			freeSize: bufferSize,
		}
		wChannel := make(chan *wBuffer, 128)

		cfile = CFile{
			Path:      path,
			OpenFlag:  flags,
			cfs:       cfs,
			Writer:    writer,
			FileSize:  tmpFileSize,
			wChannel:  wChannel,
			wBuffer:   tmpBuffer,
			chunks:    chunkInfos,
			ReaderMap: make(map[fuse.HandleID]*ReaderInfo),
		}

		go cfile.flushChannel()
	}
	return 0, &cfile
}

func (cfs *CFS) UpdateOpenFile(cfile *CFile, flags int) int32 {

	if (flags&O_WRONLY) != 0 || (flags&O_RDWR) != 0 {

		if (flags & O_APPEND) != 0 {
			chunkInfos := make([]*mp.ChunkInfo, 0)

			var ret int32
			if ret, chunkInfos = cfs.GetFileChunks(cfile.Path); ret != 0 {
				return ret
			}
			lastChunk := chunkInfos[len(chunkInfos)-1]
			tmpBuffer := wBuffer{
				buffer:    new(bytes.Buffer),
				freeSize:  bufferSize - (lastChunk.ChunkSize % bufferSize),
				chunkInfo: lastChunk,
			}
			cfile.wBuffer = tmpBuffer

			//go cfile.flushChannel()

		} else {
			cfs.DeleteFile(cfile.Path)
			if ret := cfs.createFile(cfile.Path); ret != 0 {
				return ret
			}

			tmpBuffer := wBuffer{
				buffer:   new(bytes.Buffer),
				freeSize: bufferSize,
			}

			cfile.wBuffer = tmpBuffer
			cfile.chunks = nil
			cfile.ReaderMap = make(map[fuse.HandleID]*ReaderInfo)

			//go cfile.flushChannel()
		}
	}
	return 0
}

func (cfs *CFS) createFile(path string) int32 {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("createFile failed,Dial to metanode fail :%v\n", err)
		return -1
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pCreateFileReq := &mp.CreateFileReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	pCreateFileAck, _ := mc.CreateFile(context.Background(), pCreateFileReq)
	if pCreateFileAck.Ret == 1 {
		return 1
	}
	if pCreateFileAck.Ret == 2 {
		return 2
	}
	if pCreateFileAck.Ret == 17 {
		return 17
	}
	return 0
}

func (cfs *CFS) DeleteFile(path string) int32 {

	ret, chunkInfos := cfs.GetFileChunks(path)
	if ret != 0 {
		return ret
	}

	for _, v1 := range chunkInfos {
		for _, v2 := range v1.BlockGroup.BlockInfos {

			addr := utils.Inet_ntoa(v2.DataNodeIP).String() + ":" + strconv.Itoa(int(v2.DataNodePort))
			conn, err := grpc.Dial(addr, grpc.WithInsecure())
			if err != nil {
				logger.Error("DeleteFile failed,Dial to datanode fail :%v\n", err)
				return -1
			}

			dc := dp.NewDataNodeClient(conn)

			dpDeleteChunkReq := &dp.DeleteChunkReq{
				ChunkID: v1.ChunkID,
				BlockID: v2.BlockID,
			}
			dpDeleteChunkAck, _ := dc.DeleteChunk(context.Background(), dpDeleteChunkReq)
			if dpDeleteChunkAck.Ret != 0 {
				//return dpDeleteChunkAck.Ret
			}
			conn.Close()
		}
	}

	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("DeleteFile failed,Dial to metanode fail :%v\n", err)
		return -1
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	mpDeleteFileReq := &mp.DeleteFileReq{
		FullPathName: path,
		VolID:        cfs.VolID,
	}
	mpDeleteFileAck, _ := mc.DeleteFile(context.Background(), mpDeleteFileReq)

	return mpDeleteFileAck.Ret

}
func (cfs *CFS) AllocateChunk(path string) (int32, *mp.ChunkInfo) {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("AllocateChunk failed,Dial to metanode fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pAllocateChunkReq := &mp.AllocateChunkReq{
		FileName: path,
		VolID:    cfs.VolID,
	}
	pAllocateChunkAck, err2 := mc.AllocateChunk(context.Background(), pAllocateChunkReq)
	if err2 != nil {
		return -1, nil
	}
	if pAllocateChunkAck.Ret != 0 {
		return pAllocateChunkAck.Ret, nil
	}

	return pAllocateChunkAck.Ret, pAllocateChunkAck.ChunkInfo
}

func (cfs *CFS) GetFileChunks(path string) (int32, []*mp.ChunkInfo) {
	conn, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("GetFileChunks failed,Dial to metanode fail :%v\n", err)
		return -1, nil
	}
	defer conn.Close()
	mc := mp.NewMetaNodeClient(conn)
	pGetFileChunksReq := &mp.GetFileChunksReq{
		FileName: path,
		VolID:    cfs.VolID,
	}
	pGetFileChunksAck, err2 := mc.GetFileChunks(context.Background(), pGetFileChunksReq)
	if err2 != nil {
		return -1, nil
	}
	if pGetFileChunksAck.Ret != 0 {
		return pGetFileChunksAck.Ret, nil
	}
	return pGetFileChunksAck.Ret, pGetFileChunksAck.ChunkInfos
}

type wBuffer struct {
	freeSize  int32         // chunk size
	chunkInfo *mp.ChunkInfo // chunk info
	buffer    *bytes.Buffer // chunk data
}

type ReaderInfo struct {
	LastOffset int64
	readBuf    []byte
	Ch         chan *bytes.Buffer
}

type CFile struct {
	cfs      *CFS
	Path     string
	OpenFlag int
	FileSize int64

	// for write
	WMutex sync.Mutex
	Writer int32
	//FirstW bool
	wBuffer       wBuffer
	wChannel      chan *wBuffer // write channel
	wg            sync.WaitGroup
	wLastDataNode [3]string

	// for read
	//lastoffset int64
	RMutex sync.Mutex
	chunks []*mp.ChunkInfo // chunkinfo
	//readBuf    []byte
	ReaderMap map[fuse.HandleID]*ReaderInfo
}

func (cfile *CFile) streamread(chunkidx int, ch chan *bytes.Buffer, offset int64, size int64) {
	for i, _ := range cfile.chunks[chunkidx].BlockGroup.BlockInfos {
		conn, err := grpc.Dial(utils.Inet_ntoa(cfile.chunks[chunkidx].BlockGroup.BlockInfos[i].DataNodeIP).String()+":"+strconv.Itoa(int(cfile.chunks[chunkidx].BlockGroup.BlockInfos[i].DataNodePort)), grpc.WithInsecure())
		if err != nil {
			logger.Error("streamread failed,Dial to datanode fail :%v\n", err)
			continue
		}

		dc := dp.NewDataNodeClient(conn)
		streamreadChunkReq := &dp.StreamReadChunkReq{
			ChunkID:  cfile.chunks[chunkidx].ChunkID,
			BlockID:  cfile.chunks[chunkidx].BlockGroup.BlockInfos[i].BlockID,
			Offset:   offset,
			Readsize: size,
		}
		stream, err := dc.StreamReadChunk(context.Background(), streamreadChunkReq)
		if err != nil {
			close(ch)
		}
		buffer := new(bytes.Buffer)
		for {
			ack, err := stream.Recv()
			if err == io.EOF {
				conn.Close()
				break
			}
			if err != nil {
				close(ch)
			}
			buffer.Write(ack.Databuf)
		}
		ch <- buffer
		conn.Close()
	}
}

func (cfile *CFile) Read(handleId fuse.HandleID, data *[]byte, offset int64, readsize int64) int64 {

	if cfile.chunks == nil || len(cfile.chunks) == 0 {
		return -1
	}

	if offset+readsize > cfile.FileSize {
		readsize = cfile.FileSize - offset
	}

	var length int64 = 0
	var free_offset int64
	var free_size int64
	var begin_chunk_num int = 0
	var end_chunk_num int = 0
	cur_offset := offset
	for i, _ := range cfile.chunks {
		free_offset = cur_offset - int64(cfile.chunks[i].ChunkSize)
		if free_offset <= 0 {
			begin_chunk_num = i
			break
		} else {
			cur_offset = free_offset
		}
	}

	cur_size := offset + readsize

	for i, _ := range cfile.chunks {
		free_size = cur_size - int64(cfile.chunks[i].ChunkSize)
		if free_size <= 0 {
			end_chunk_num = i
			break
		} else {
			cur_size = free_size
		}
	}

	var each_read_len int64
	freesize := readsize
	if end_chunk_num < begin_chunk_num {
		logger.Error("This Read data from beginchunk:%v lager than endchunk:%v\n", begin_chunk_num, end_chunk_num)
		return -1
	}

	if begin_chunk_num > len(cfile.chunks) || end_chunk_num+1 > len(cfile.chunks) || begin_chunk_num > cap(cfile.chunks) || end_chunk_num+1 > cap(cfile.chunks) {
		return -1
	}

	for i, _ := range cfile.chunks[begin_chunk_num : end_chunk_num+1] {
		index := i + begin_chunk_num
		if cur_offset+freesize < int64(cfile.chunks[index].ChunkSize) {
			each_read_len = freesize
		} else {
			each_read_len = int64(cfile.chunks[index].ChunkSize) - cur_offset
		}
		if len(cfile.ReaderMap[handleId].readBuf) == 0 {
			buffer := new(bytes.Buffer)
			cfile.ReaderMap[handleId].Ch = make(chan *bytes.Buffer)
			go cfile.streamread(index, cfile.ReaderMap[handleId].Ch, 0, int64(cfile.chunks[index].ChunkSize))
			buffer = <-cfile.ReaderMap[handleId].Ch
			cfile.ReaderMap[handleId].readBuf = buffer.Next(buffer.Len())
			buffer.Reset()
			buffer = nil
		}

		/*var ch chan *bytes.Buffer
		//ch = make(chan *bytes.Buffer)
		//go cfile.streamread(index, ch, cur_offset, each_read_len)
		//buffer = <-ch
		*data = append(*data, buffer.Next(buffer.Len())...)
		buffer.Reset()*/

		buflen := int64(len(cfile.ReaderMap[handleId].readBuf))
		bufcap := int64(cap(cfile.ReaderMap[handleId].readBuf))

		if cur_offset > buflen || cur_offset > bufcap || cur_offset+each_read_len > buflen || cur_offset+each_read_len > bufcap {
			logger.Error("== Read chunk:%v from datanode (offset:%v -- needreadsize:%v) lager than exist (buflen:%v -- bufcap:%v)\n", index, cur_offset, each_read_len, buflen, bufcap)
			return -1
		}
		*data = append(*data, cfile.ReaderMap[handleId].readBuf[cur_offset:cur_offset+each_read_len]...)
		cur_offset += each_read_len
		if cur_offset == int64(len(cfile.ReaderMap[handleId].readBuf)) {
			//if cur_offset == int64(cfile.chunks[index].ChunkSize) {
			cur_offset = 0
			cfile.ReaderMap[handleId].readBuf = []byte{}
		}
		freesize = freesize - each_read_len
		length += each_read_len
	}
	return length
}

/*func (cfile *CFile) Seek(offset int64, whence int32) int64 {
	if whence <= 1 {
		cfile.lastoffset = offset
	} else if whence == 2 {
		cfile.lastoffset = cfile.FileSize - offset
	} else {
		logger.Error("Seek file for read/write invalid whence:%v\n", whence)
		return -1
	}
	return 0
}
*/

func (cfile *CFile) Write(buf []byte, len int32) int32 {
	cfile.WMutex.Lock()
	defer cfile.WMutex.Unlock()
	var w int32
	w = 0

	for w < len {
		if (cfile.FileSize % chunkSize) == 0 {
			logger.Debug("need a new chunk...")
			var ret int32
			ret, cfile.wBuffer.chunkInfo = cfile.cfs.AllocateChunk(cfile.Path)
			if ret != 0 {
				if ret == 28 /*ENOSPC*/ {
					return -1
				} else {
					return -2
				}
			}

		}
		if cfile.wBuffer.freeSize == 0 {
			cfile.wBuffer.buffer = new(bytes.Buffer)
			cfile.wBuffer.freeSize = bufferSize
		}
		if len-w < cfile.wBuffer.freeSize {
			cfile.wBuffer.buffer.Write(buf[w:len])
			cfile.wBuffer.freeSize = cfile.wBuffer.freeSize - (len - w)
			cfile.FileSize = cfile.FileSize + int64(len-w)
			cfile.wBuffer.chunkInfo.ChunkSize = cfile.wBuffer.chunkInfo.ChunkSize + int32(len-w)
			w = len
			break
		} else {
			cfile.wBuffer.buffer.Write(buf[w : w+cfile.wBuffer.freeSize])
			w = w + cfile.wBuffer.freeSize
			cfile.FileSize = cfile.FileSize + int64(cfile.wBuffer.freeSize)
			cfile.wBuffer.chunkInfo.ChunkSize = cfile.wBuffer.chunkInfo.ChunkSize + int32(cfile.wBuffer.freeSize)
			cfile.wBuffer.freeSize = 0
		}

		if cfile.wBuffer.freeSize == 0 {
			cfile.push2Channel()
		}
	}

	return w
}

func (cfile *CFile) push2Channel() int32 {
	if cfile.wBuffer.chunkInfo == nil {
		return 0
	}
	wBuffer := cfile.wBuffer   // record cur buffer
	cfile.wChannel <- &wBuffer // push to channel
	cfile.wg.Add(1)
	return 0
}

func (cfile *CFile) close2Channel() int32 {
	/*
	   1. avoid repeat push for integer file ETC. 64MB , the last push has already done in Write func
	*/
	if cfile.wBuffer.freeSize != 0 && cfile.wBuffer.chunkInfo != nil {
		wBuffer := cfile.wBuffer   // record cur buffer
		cfile.wChannel <- &wBuffer // push to channel
		cfile.wg.Add(1)
	}
	return 0
}

func (cfile *CFile) DestroyChannel() {
	var wEndBuffer wBuffer
	wEndBuffer.buffer = nil
	wEndBuffer.freeSize = 0
	wEndBuffer.chunkInfo = nil

	cfile.wChannel <- &wEndBuffer // send the end flag to channel
	cfile.wg.Add(1)
}

func (cfile *CFile) flushChannel() {
	var addr [3]string
	var connD [3]*grpc.ClientConn
	var dc [3]dp.DataNodeClient

	// update chunkinfo to metanode
	connM, err := grpc.Dial(MetaNodeAddr, grpc.WithInsecure())
	if err != nil {
		logger.Error("flushChannel,Dial failed:%v\n", err)
	}
	defer connM.Close()

	for {
		v := <-cfile.wChannel

		if v.buffer == nil && v.chunkInfo == nil && v.freeSize == 0 {
			if connD[0] != nil {
				connD[0].Close()
			}
			if connD[1] != nil {
				connD[1].Close()
			}
			if connD[2] != nil {
				connD[2].Close()
			}
			cfile.wg.Add(-1)
			break
		}

		dataBuf := v.buffer.Next(v.buffer.Len())

		for i := range v.chunkInfo.BlockGroup.BlockInfos {
			addr[i] = utils.Inet_ntoa(v.chunkInfo.BlockGroup.BlockInfos[i].DataNodeIP).String() + ":" + strconv.Itoa(int(v.chunkInfo.BlockGroup.BlockInfos[i].DataNodePort))
			if addr[i] != cfile.wLastDataNode[i] {
				if connD[i] != nil {
					connD[i].Close()
				}
				var err error
				connD[i], err = grpc.Dial(addr[i], grpc.WithInsecure())
				if err != nil {
					logger.Error("flushChannel to datanode failed,Dial failed:%v\n", err)
					return
				}
				dc[i] = dp.NewDataNodeClient(connD[i])
				cfile.wLastDataNode[i] = addr[i]
			}

			blockID := v.chunkInfo.BlockGroup.BlockInfos[i].BlockID
			chunkID := v.chunkInfo.ChunkID

			pWriteChunkReq := &dp.WriteChunkReq{
				ChunkID: chunkID,
				BlockID: blockID,
				Databuf: dataBuf,
			}
			pWriteChunkAck, _ := dc[i].WriteChunk(context.Background(), pWriteChunkReq)
			if pWriteChunkAck.Ret != 0 {
				logger.Error("flushChannel WriteChunk Failed :%v\n", pWriteChunkAck.Ret)
				cfile.cfs.Status = 1
				return
			}

		}
		mc := mp.NewMetaNodeClient(connM)
		pSyncChunkReq := &mp.SyncChunkReq{
			FileName:  cfile.Path,
			VolID:     cfile.cfs.VolID,
			ChunkInfo: v.chunkInfo,
		}

		pSyncChunkAck, _ := mc.SyncChunk(context.Background(), pSyncChunkReq)
		if pSyncChunkAck.Ret != 0 {
			cfile.cfs.Status = 1
			return
		}

		chunkNum := len(cfile.chunks)

		if chunkNum == 0 {
			cfile.chunks = append(cfile.chunks, v.chunkInfo)
		} else {
			if cfile.chunks[chunkNum-1].ChunkID == v.chunkInfo.ChunkID {
				cfile.chunks[chunkNum-1].ChunkSize = v.chunkInfo.ChunkSize
			} else {
				cfile.chunks = append(cfile.chunks, v.chunkInfo)
			}
		}

		cfile.wg.Add(-1)
	}
}

func (cfile *CFile) Flush() int32 {
	return 0
}

func (cfile *CFile) Sync() int32 {
	return 0
}

//func closeP(cfile *CFile) {
//cfile = nil
//}
func (cfile *CFile) Close(flags int) int32 {
	if (flags&O_WRONLY) != 0 || (flags&O_RDWR) != 0 {
		cfile.close2Channel()
		cfile.wg.Wait()
	}
	return 0
}

func ProcessLocalBuffer(buffer []byte, cfile *CFile) {
	cfile.Write(buffer, int32(len(buffer)))
}

func ReadLocalAndWriteCFS(filePth string, bufSize int, hookfn func([]byte, *CFile), cfile *CFile) error {
	f, err := os.Open(filePth)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, bufSize)
	bfRd := bufio.NewReader(f)
	for {
		n, err := bfRd.Read(buf)
		hookfn(buf[:n], cfile)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	return nil
}
