package app;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@SpringBootApplication
@RestController
public class Application {

    @GetMapping(value = "/", produces = MediaType.TEXT_PLAIN_VALUE)
    public String root() {
        return "hello\n";
    }

    @GetMapping(value = "/api/hello", produces = MediaType.APPLICATION_JSON_VALUE)
    public String hello() {
        // Exact bytes; returned as-is (String message converter, no re-encoding).
        return "{\"msg\":\"hello from spring-boot\"}";
    }

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }
}
