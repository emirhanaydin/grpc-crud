package main

import (
	"context"
	"crud/blog/pb"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"log"
	"os"
	"time"
)

const PORT = "50051"
const TIMEOUT = 1 * time.Second

var client pb.BlogServiceClient
var blogResult *pb.Blog

func main() {
	address := "localhost:" + PORT

	certFile := os.Getenv("SSL_CERT_FILE")

	if certFile == "" {
		log.Fatal("SSL_CERT_FILE environment variable is not set")
	}

	creds, err := credentials.NewClientTLSFromFile(certFile, "")
	if err != nil {
		log.Fatalf("error creating server TLS:\n\n%v\n", err)
	}

	cc, err := grpc.Dial(address, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("can not connect to %s:\n%e\n", address, err)
	}
	defer func() {
		err := cc.Close()
		if err != nil {
			log.Fatalf("can not close connection:\n%e\n", err)
		}
	}()

	client = pb.NewBlogServiceClient(cc)

	blogResult = createBlog()
	readBlog()
	blogResult = updateBlog()
	deleteBlog()
	listBlog()
}

func createBlog() *pb.Blog {
	blog := &pb.Blog{
		AuthorId: "this is a sample ID",
		Title:    "This is a Sample Blog Post",
		Content:  "This is some content for the sample blog post.",
	}

	request := &pb.CreateBlogRequest{Blog: blog}

	ctx, cancelFunc := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancelFunc()

	response, err := client.CreateBlog(ctx, request)
	if err != nil {
		s, ok := status.FromError(err)

		if ok {
			switch s.Code() {
			case codes.DeadlineExceeded:
				log.Fatalf("request timeout")
			default:
				log.Fatal(s.Message())
			}
		} else {
			log.Fatalf("error invoking remote call createBlog:\n%v\n", err)
		}
	}

	blog.Id = response.Id
	fmt.Printf("blog has been created:\n%v\n", blog)

	return blog
}

func readBlog() {
	id := blogResult.GetId()

	request := &pb.ReadBlogRequest{Id: id}

	ctx, cancelFunc := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancelFunc()

	response, err := client.ReadBlog(ctx, request)
	if err != nil {
		s, ok := status.FromError(err)

		if ok {
			switch s.Code() {
			case codes.DeadlineExceeded:
				log.Fatalf("request timeout")
			default:
				log.Fatal(s.Message())
			}
		} else {
			log.Fatalf("error invoking remote call createBlog:\n%v\n", err)
		}
	}

	blog := response.GetBlog()

	fmt.Printf("blog has been read:\n%v\n", blog)
}

func updateBlog() *pb.Blog {
	blog := blogResult
	blog.AuthorId = "Updated Author ID"
	blog.Title = "This is an Updated Title"
	blog.Content = "The content was also updated."

	request := &pb.UpdateBlogRequest{Blog: blog}

	ctx, cancelFunc := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancelFunc()

	_, err := client.UpdateBlog(ctx, request)
	if err != nil {
		s, ok := status.FromError(err)

		if ok {
			switch s.Code() {
			case codes.DeadlineExceeded:
				log.Fatalf("request timeout")
			default:
				log.Fatal(s.Message())
			}
		} else {
			log.Fatalf("error invoking remote call createBlog:\n%v\n", err)
		}
	}

	fmt.Printf("blog has been updated:\n%v\n", blog)
	return blog
}

func deleteBlog() *pb.Blog {
	blog := blogResult

	request := &pb.DeleteBlogRequest{Id: blog.GetId()}

	ctx, cancelFunc := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancelFunc()

	_, err := client.DeleteBlog(ctx, request)
	if err != nil {
		s, ok := status.FromError(err)

		if ok {
			switch s.Code() {
			case codes.DeadlineExceeded:
				log.Fatalf("request timeout")
			default:
				log.Fatal(s.Message())
			}
		} else {
			log.Fatalf("error invoking remote call createBlog:\n%v\n", err)
		}
	}

	fmt.Printf("blog has been deleted:\n%v\n", blog)
	return blog
}

func listBlog() {
	stream, err := client.ListBlog(context.TODO(), &emptypb.Empty{})
	if err != nil {
		log.Fatalf("error while creating stream:\n%v\n", err)
	}

	fmt.Println("listing blogs:")

	waitc := make(chan struct{})

	go func() {
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				s, ok := status.FromError(err)
				if ok {
					switch s.Code() {
					case codes.DeadlineExceeded:
						log.Fatalf("request timeout")
					default:
						log.Fatal(s.Message())
					}
				} else {
					log.Fatalf("error invoking remote stream listBlog:\n%v\n", err)
				}
			}
			fmt.Println(response.GetBlog())
		}

		close(waitc)
	}()

	<-waitc
}
