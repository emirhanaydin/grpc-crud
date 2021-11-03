package main

import (
	"context"
	"crud/blog/pb"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"
)

var mongoClient *mongo.Client
var blogCollection *mongo.Collection

const network = "tcp"
const PORT = "50051"

type server struct {
	pb.UnimplementedBlogServiceServer
}

func newServer() *server {
	return &server{}
}

type blogItem struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	AuthorID string             `bson:"author_id"`
	Content  string             `bson:"content"`
	Title    string             `bson:"title"`
}

func itemFromBlog(blog *pb.Blog) *blogItem {
	return &blogItem{
		AuthorID: blog.GetAuthorId(),
		Content:  blog.GetContent(),
		Title:    blog.GetTitle(),
	}
}

func blogFromItem(item *blogItem) *pb.Blog {
	return &pb.Blog{
		Id:       item.ID.Hex(),
		AuthorId: item.AuthorID,
		Title:    item.Title,
		Content:  item.Content,
	}
}

func (s *server) CreateBlog(_ context.Context, in *pb.CreateBlogRequest) (*pb.CreateBlogResponse, error) {
	blog := in.GetBlog()

	data := itemFromBlog(blog)

	result, err := blogCollection.InsertOne(context.TODO(), data)
	if err != nil {
		log.Printf("internal error:\n%v\n", err)
		return nil, status.Errorf(codes.Internal, "internal server error")
	}
	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return nil, status.Errorf(codes.Internal, "can not cast to ObjectID")
	}

	response := &pb.CreateBlogResponse{Id: id.Hex()}

	return response, nil
}

func (s *server) ReadBlog(_ context.Context, in *pb.ReadBlogRequest) (*pb.ReadBlogResponse, error) {
	id, err := primitive.ObjectIDFromHex(in.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "given ID is invalid")
	}

	filter := bson.D{{"_id", id}}
	var result blogItem
	err = blogCollection.FindOne(context.TODO(), filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "blog post with given ID is not found")
		}

		log.Printf("internal error:\n%v\n", err)
		return nil, status.Errorf(codes.Internal, "internal server error")
	}

	blog := blogFromItem(&result)
	response := &pb.ReadBlogResponse{Blog: blog}

	return response, nil
}

func (s *server) UpdateBlog(_ context.Context, in *pb.UpdateBlogRequest) (*emptypb.Empty, error) {
	blog := in.GetBlog()

	id, err := primitive.ObjectIDFromHex(blog.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "given ID is invalid")
	}

	data := itemFromBlog(blog)

	filter := bson.D{{"_id", id}}
	update, _ := bson.Marshal(data)

	_, err = blogCollection.ReplaceOne(context.TODO(), filter, update)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "blog post with given ID is not found")
		}

		log.Printf("internal error:\n%v\n", err)
		return nil, status.Errorf(codes.Internal, "internal server error")
	}

	return &emptypb.Empty{}, nil
}

func (s *server) DeleteBlog(_ context.Context, in *pb.DeleteBlogRequest) (*emptypb.Empty, error) {
	id, err := primitive.ObjectIDFromHex(in.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "given ID is invalid")
	}

	filter := bson.D{{"_id", id}}
	result, err := blogCollection.DeleteOne(context.TODO(), filter)
	if result.DeletedCount < 1 {
		err = mongo.ErrNoDocuments
	}
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "blog post with given ID is not found")
		}

		log.Printf("internal error:\n%v\n", err)
		return nil, status.Errorf(codes.Internal, "internal server error")
	}

	return &emptypb.Empty{}, nil
}

func (s *server) ListBlog(_ *emptypb.Empty, stream pb.BlogService_ListBlogServer) error {
	filter := bson.D{}
	cur, err := blogCollection.Find(context.TODO(), filter)
	if err != nil {
		log.Printf("internal error:\n%v\n", err)
		return status.Errorf(codes.Internal, "internal server error")
	}
	defer func() {
		err := cur.Close(context.TODO())
		if err != nil {
			log.Printf("internal error:\n%v\n", err)
		}
	}()

	for cur.Next(context.TODO()) {
		data := &blogItem{}
		err := cur.Decode(data)
		if err != nil {
			log.Printf("internal error:\n%v\n", err)
			return status.Errorf(codes.Internal, "internal server error")
		}

		err = stream.Send(&pb.ListBlogResponse{Blog: blogFromItem(data)})
		if err != nil {
			log.Printf("internal error:\n%v\n", err)
			return status.Errorf(codes.Internal, "internal server error")
		}
	}

	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rand.Seed(time.Now().UnixNano())

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		fatalEnv("APP_ENV")
	}

	connectMongoDb()

	network := network
	address := fmt.Sprintf("0.0.0.0:%s", PORT)

	listener, err := net.Listen(network, address)
	if err != nil {
		log.Fatalf("can not listen on network %s at address %s:\n%v", network, address, err)
	}

	certFile := os.Getenv("SSL_CERT_FILE")
	keyFile := os.Getenv("SSL_KEY_FILE")

	if certFile == "" {
		fatalEnv("SSL_CERT_FILE")
	}
	if keyFile == "" {
		fatalEnv("SSL_KEY_FILE")
	}

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)

	if err != nil {
		var msgBuilder = strings.Builder{}
		msgBuilder.WriteString(fmt.Sprintf("error creating server TLS:\n\n%v\n", err))

		if errors.Is(err, fs.ErrNotExist) {
			msgBuilder.WriteString(fmt.Sprintf("certificate file: %s\nkey file: %s\n", certFile, keyFile))
		}

		log.Fatal(msgBuilder.String())
	}

	s := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterBlogServiceServer(s, newServer())

	if isDevelopment(appEnv) {
		reflection.Register(s)
	}

	fmt.Printf("starting server on %s at address %s\n", network, address)

	go func() {
		if err := s.Serve(listener); err != nil {
			log.Fatalf("can not serve:\n%v", err)
		}
	}()

	mongoClient = connectMongoDb()
	defer func() {
		if err := mongoClient.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	<-ch

	fmt.Println("stopping the server")
	s.Stop()
}

func connectMongoDb() *mongo.Client {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		fatalEnv("MONGODB_URI")
	}

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("error connecting MongDB:\n%v\n", err)
	}

	blogCollection = client.Database("blog").Collection("post")

	return client
}

func fatalEnv(key string) {
	log.Fatalf("%s environment variable is not set", key)
}

func isDevelopment(appEnv string) bool {
	return appEnv == "development"
}
