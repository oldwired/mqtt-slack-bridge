# Readme

Esay Slack/MQTT Coupling

## Function

All messages posted to a specific slack channel get published to a specified MQTT topic and vice versa.  
This program is a platform to explore MQTT and Slack-API technologies.

## Sample Config

`config.json`

```javascript
{
    "secret": "REDACTED",
    "channel": "#testing",
    "topic4channel": "test/chat",
    "debug": false,
    "broker": "127.0.0.1",
    "port": "1883",
    "clientID": "mqttbot",
}
```
